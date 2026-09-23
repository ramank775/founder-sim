// Package web is the htmx UI. Every handler is thin: resolve the player,
// load the run, call game.Service, render a template or fragment.
package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ramank775/founder-sim/bridge"
	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/internal/auth"
	"github.com/ramank775/founder-sim/internal/game"
	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/internal/store"
	"github.com/ramank775/founder-sim/plugin"
)

//go:embed templates/*.html
var tmplFS embed.FS

type Server struct {
	Auth   *auth.Auth
	Game   *game.Service
	Cipher *auth.Cipher
	Hub    *bridge.Hub // reverse tunnel to services on players' machines
	Dev    bool        // shows raw model output and the magic link inline
	tmpl   *template.Template
	mux    *http.ServeMux
}

type ctxKey int

const playerKey ctxKey = 1

func New(a *auth.Auth, g *game.Service, c *auth.Cipher, hub *bridge.Hub, dev bool) (*Server, error) {
	funcs := template.FuncMap{
		"rupees": func(n int64) string { return "₹" + commas(n) },
		"safe":   func(s string) template.HTML { return template.HTML(sanitizeHTML(s)) },
		"deref":  func(p *int64) int64 { return *p },
		"list":   func(xs ...int) []int { return xs },
		"nl2p": func(s string) template.HTML {
			var b strings.Builder
			for _, para := range strings.Split(strings.TrimSpace(s), "\n\n") {
				b.WriteString("<p>" + template.HTMLEscapeString(strings.TrimSpace(para)) + "</p>")
			}
			return template.HTML(b.String())
		},
	}
	t, err := template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{Auth: a, Game: g, Cipher: c, Hub: hub, Dev: dev, tmpl: t, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /{$}", s.withPlayer(s.home, false))
	m.HandleFunc("POST /login", s.login)
	m.HandleFunc("GET /login/{token}", s.loginComplete)
	m.HandleFunc("POST /logout", s.logout)
	m.HandleFunc("GET /settings", s.withPlayer(s.settings, true))
	m.HandleFunc("POST /settings", s.withPlayer(s.settingsSave, true))
	m.HandleFunc("POST /settings/bridge-token", s.withPlayer(s.bridgeToken, true))
	m.Handle("/bridge/", bridge.Handler(s.Hub, s.Game.Store))
	m.HandleFunc("GET /new", s.withPlayer(s.newRun, true))
	m.HandleFunc("POST /new", s.withPlayer(s.newRunSubmit, true))
	m.HandleFunc("GET /run/{id}", s.withRun(s.runPage))
	m.HandleFunc("POST /run/{id}/act", s.withRun(s.act))
	m.HandleFunc("POST /run/{id}/balance", s.withRun(s.balance))
	m.HandleFunc("POST /run/{id}/pin", s.withRun(s.pin))
	m.HandleFunc("POST /run/{id}/todo/{todo}/{what}", s.withRun(s.todo))
	m.HandleFunc("POST /run/{id}/tool/{plugin}/{tool}", s.withRun(s.tool))
	m.HandleFunc("POST /run/{id}/exit", s.withRun(s.exit))
	m.HandleFunc("GET /run/{id}/dev", s.withRun(s.devState))
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
}

// ---------- helpers ----------

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		s.render(w, "error.html", map[string]any{"Error": msg})
		return
	}
	http.Error(w, msg, status)
}

func playerFrom(r *http.Request) *store.Player {
	p, _ := r.Context().Value(playerKey).(*store.Player)
	return p
}

func (s *Server) withPlayer(h http.HandlerFunc, required bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p *store.Player
		if c, err := r.Cookie(auth.CookieName); err == nil {
			p, _ = s.Auth.Resolve(r.Context(), c.Value)
		}
		if p == nil && required {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), playerKey, p)))
	}
}

type runHandler func(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord)

func (s *Server) withRun(h runHandler) http.HandlerFunc {
	return s.withPlayer(func(w http.ResponseWriter, r *http.Request) {
		p := playerFrom(r)
		rec, err := s.Game.Store.GetRun(r.Context(), r.PathValue("id"))
		if err != nil || rec.PlayerID != p.ID {
			s.fail(w, r, http.StatusNotFound, "no such run")
			return
		}
		h(w, r, p, rec)
	}, true)
}

func (s *Server) llmConfig(p *store.Player) (llm.Config, error) {
	key, err := s.Cipher.Decrypt(p.APIKeyEnc)
	if err != nil {
		return llm.Config{}, err
	}
	cfg := llm.Config{BaseURL: p.LLMBaseURL, Model: p.LLMModel, APIKey: key, JSONMode: p.LLMJSONMode}
	// "bridge://llm": the model lives on the player's machine; route through their bridge.
	if svc, ok := bridge.ServiceFromURL(cfg.BaseURL); ok {
		cfg.HTTP = s.Hub.HTTPClient(p.ID, svc)
	}
	return cfg, nil
}

func (s *Server) settingsData(p *store.Player, msg string) map[string]any {
	return map[string]any{
		"Player": p, "HasKey": len(p.APIKeyEnc) > 0, "Message": msg,
		"BridgeConnected": s.Hub.Connected(p.ID), "BaseURL": s.Auth.BaseURL,
	}
}

func commas(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	// Indian grouping: last 3, then 2s.
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		var parts []string
		for len(head) > 2 {
			parts = append([]string{head[len(head)-2:]}, parts...)
			head = head[:len(head)-2]
		}
		if head != "" {
			parts = append([]string{head}, parts...)
		}
		s = strings.Join(parts, ",") + "," + tail
	}
	if neg {
		return "-" + s
	}
	return s
}

// ---------- auth pages ----------

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	if p == nil {
		s.render(w, "login.html", map[string]any{"Dev": s.Dev})
		return
	}
	runs, _ := s.Game.Store.ListRuns(r.Context(), p.ID)
	s.render(w, "home.html", map[string]any{"Player": p, "Runs": runs, "NeedsLLM": p.LLMBaseURL == ""})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	url, err := s.Auth.Begin(r.Context(), r.FormValue("email"))
	if err != nil {
		s.render(w, "login.html", map[string]any{"Error": err.Error(), "Dev": s.Dev})
		return
	}
	s.render(w, "login.html", map[string]any{"Sent": true, "Link": url, "Dev": s.Dev})
}

func (s *Server) loginComplete(w http.ResponseWriter, r *http.Request) {
	_, tok, err := s.Auth.Complete(r.Context(), r.PathValue("token"))
	if err != nil {
		s.render(w, "login.html", map[string]any{"Error": err.Error(), "Dev": s.Dev})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(30 * 24 * time.Hour)})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = s.Auth.Store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---------- settings ----------

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	s.render(w, "settings.html", s.settingsData(playerFrom(r), ""))
}

// bridgeToken issues (or rotates) the player's bridge token.
func (s *Server) bridgeToken(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	if r.FormValue("revoke") == "1" {
		p.BridgeToken = ""
	} else {
		p.BridgeToken = bridge.NewToken()
	}
	if err := s.Game.Store.PutPlayer(r.Context(), p); err != nil {
		s.fail(w, r, 500, err.Error())
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) settingsSave(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	p.LLMBaseURL = strings.TrimSpace(r.FormValue("base_url"))
	p.LLMModel = strings.TrimSpace(r.FormValue("model"))
	if k := strings.TrimSpace(r.FormValue("api_key")); k != "" {
		enc, err := s.Cipher.Encrypt(k)
		if err != nil {
			s.fail(w, r, 500, err.Error())
			return
		}
		p.APIKeyEnc = enc
	}
	msg := "Saved."
	if r.FormValue("probe") == "1" && p.LLMBaseURL != "" {
		cfg, err := s.llmConfig(p)
		if err == nil {
			cfg, err = llm.Probe(r.Context(), cfg)
		}
		if err != nil {
			msg = "Saved, but the endpoint did not answer: " + err.Error()
		} else {
			p.LLMJSONMode = cfg.JSONMode
			msg = fmt.Sprintf("Saved. Endpoint answered. JSON mode: %v.", cfg.JSONMode)
		}
	}
	if err := s.Game.Store.PutPlayer(r.Context(), p); err != nil {
		s.fail(w, r, 500, err.Error())
		return
	}
	s.render(w, "settings.html", s.settingsData(p, msg))
}

// ---------- new run ----------

func (s *Server) newRun(w http.ResponseWriter, r *http.Request) {
	s.render(w, "new.html", map[string]any{"Player": playerFrom(r), "Plugins": s.Game.Plugins.Manifests()})
}

func (s *Server) newRunSubmit(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	cfg, err := s.llmConfig(p)
	if err != nil {
		s.fail(w, r, 500, err.Error())
		return
	}
	age, _ := strconv.Atoi(r.FormValue("age"))
	savings, _ := strconv.ParseInt(strings.ReplaceAll(r.FormValue("savings"), ",", ""), 10, 64)
	var obs []engine.Obligation
	for i := 1; i <= 4; i++ {
		label := strings.TrimSpace(r.FormValue(fmt.Sprintf("ob_label_%d", i)))
		amt, _ := strconv.ParseInt(strings.ReplaceAll(r.FormValue(fmt.Sprintf("ob_amount_%d", i)), ",", ""), 10, 64)
		if label != "" && amt > 0 {
			obs = append(obs, engine.Obligation{Label: label, Amount: amt})
		}
	}
	scale, _ := strconv.ParseInt(r.FormValue("time_scale_ms"), 10, 64)
	in := game.StartInput{
		JobDescription: r.FormValue("job"), Idea: r.FormValue("idea"), Age: age,
		MaritalStatus: r.FormValue("marital"), Kids: r.FormValue("kids"), Savings: savings, Obligations: obs,
		Technical: r.FormValue("technical") == "yes", TimeScaleMs: scale, Plugins: r.Form["plugins"],
	}
	rec, err := s.Game.Start(r.Context(), p, cfg, in)
	if err != nil {
		s.render(w, "new.html", map[string]any{"Player": p, "Plugins": s.Game.Plugins.Manifests(), "Error": err.Error()})
		return
	}
	http.Redirect(w, r, "/run/"+rec.Run.ID, http.StatusSeeOther)
}

// ---------- run ----------

type toolView struct {
	Plugin string
	Spec   plugin.ToolSpec
	HTML   template.HTML
}

func (s *Server) runData(r *http.Request, p *store.Player, rec *store.RunRecord, extra map[string]any) map[string]any {
	run := rec.Run
	var tools []toolView
	for _, name := range run.Plugins {
		pl, m, ok := s.Game.Plugins.Get(name)
		if !ok || !m.Has(plugin.CapTools) {
			continue
		}
		specs, _ := pl.Tools(r.Context())
		for _, sp := range specs {
			tools = append(tools, toolView{Plugin: name, Spec: sp})
		}
	}
	d := map[string]any{
		"Player": p, "Run": run, "View": run.View(), "Tools": tools, "Dev": s.Dev,
		"Log": lastN(run.Log, 40),
	}
	for k, v := range extra {
		d[k] = v
	}
	return d
}

func lastN(l []engine.LogEntry, n int) []engine.LogEntry {
	if len(l) <= n {
		return l
	}
	return l[len(l)-n:]
}

func (s *Server) runPage(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	if _, err := s.Game.Refresh(r.Context(), rec); err != nil {
		log.Printf("refresh: %v", err)
	}
	s.render(w, "run.html", s.runData(r, p, rec, nil))
}

func (s *Server) act(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	cfg, err := s.llmConfig(p)
	if err != nil {
		s.fail(w, r, 500, err.Error())
		return
	}
	res, err := s.Game.Turn(r.Context(), p, cfg, rec, r.FormValue("action"))
	if err != nil {
		s.fail(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.render(w, "frag_turn.html", s.runData(r, p, rec, map[string]any{"Turn": res}))
}

func (s *Server) balance(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	money, cost := rec.Run.CheckBalance()
	_ = s.Game.Store.PutRun(r.Context(), rec)
	s.render(w, "frag_turn.html", s.runData(r, p, rec, map[string]any{"Balance": money, "BalanceCost": cost, "ShowBalance": true}))
}

func (s *Server) pin(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	op := "pin"
	if r.FormValue("unpin") == "1" {
		op = "unpin"
	}
	key := r.FormValue("key")
	if key == "" {
		key = "money"
	}
	rec.Run.Apply(engine.Proposal{Deltas: []engine.Delta{{Op: op, Key: key}}}, engine.Source{Kind: "engine"})
	_ = s.Game.Store.PutRun(r.Context(), rec)
	s.render(w, "frag_turn.html", s.runData(r, p, rec, nil))
}

func (s *Server) todo(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	id, what := r.PathValue("todo"), r.PathValue("what")
	for i := range rec.Run.Todos {
		if rec.Run.Todos[i].ID == id {
			switch what {
			case "done":
				rec.Run.Todos[i].Done = true
			case "dismiss":
				rec.Run.Todos[i].Dismissed = true
				// Dismissing is ignoring. The engine remembers.
				rec.Run.Missed = append(rec.Run.Missed, engine.MissedThing{Day: rec.Run.CurrentDay, What: rec.Run.Todos[i].Text})
			}
		}
	}
	_ = s.Game.Store.PutRun(r.Context(), rec)
	s.render(w, "frag_turn.html", s.runData(r, p, rec, nil))
}

func (s *Server) tool(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	name, tool := r.PathValue("plugin"), r.PathValue("tool")
	res, err := s.Game.InvokeTool(r.Context(), rec, name, tool, r.FormValue("input"))
	if err != nil {
		if errors.Is(err, plugin.ErrNoSuchTool) {
			s.fail(w, r, 404, "no such tool")
			return
		}
		s.fail(w, r, 400, err.Error())
		return
	}
	body := res.HTML
	if body == "" {
		body = "<p>" + template.HTMLEscapeString(res.Text) + "</p>"
	}
	s.render(w, "frag_panel.html", map[string]any{"Plugin": name, "Tool": tool, "Body": template.HTML(sanitizeHTML(body)), "RunID": rec.Run.ID})
}

func (s *Server) exit(w http.ResponseWriter, r *http.Request, p *store.Player, rec *store.RunRecord) {
	if err := s.Game.Exit(r.Context(), rec, r.FormValue("kind"), r.FormValue("note")); err != nil {
		s.fail(w, r, 400, err.Error())
		return
	}
	http.Redirect(w, r, "/run/"+rec.Run.ID, http.StatusSeeOther)
}

func (s *Server) devState(w http.ResponseWriter, r *http.Request, _ *store.Player, rec *store.RunRecord) {
	if !s.Dev {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, rec)
}
