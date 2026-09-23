package web

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ramank775/founder-sim/bridge"
	"github.com/ramank775/founder-sim/internal/llm"
)

// A fake Ollama on "the player's machine": OpenAI-compatible, returns a
// valid turn proposal. It must only ever be reached through the bridge.
func fakeOllama(t *testing.T, hits *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		var req struct {
			Messages []llm.Message `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1].Content
		content := `{"narration":"Narrated on your own GPU.","slot_cost":1}`
		if strings.Contains(last, `"segment"`) {
			content = `{"segment":"b2c"}`
		}
		if strings.Contains(last, `{"ok":true}`) {
			content = `{"ok":true}`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "llama3.1", "choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
	}))
}

func TestTurnThroughBridgeToLocalModel(t *testing.T) {
	ts, g := newTestServer(t)
	defer ts.Close()
	// Real OpenAI-compatible client this time; the fake model is the local one.
	g.NewClient = func(cfg llm.Config) llm.Client { return llm.NewOpenAICompat(cfg) }

	hits := 0
	local := fakeOllama(t, &hits)
	defer local.Close()

	c := client(t)
	_, body := post(t, c, ts.URL+"/login", url.Values{"email": {"gpu@example.com"}}, false)
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ts.URL) + `/login/[a-f0-9]+)"`).FindStringSubmatch(body)
	get(t, c, m[1])

	// Create a bridge token from Settings and read it off the page.
	post(t, c, ts.URL+"/settings/bridge-token", nil, false)
	_, body = get(t, c, ts.URL+"/settings")
	tm := regexp.MustCompile(`-token ([a-f0-9]{48})`).FindStringSubmatch(body)
	if tm == nil {
		t.Fatalf("no bridge token on settings page:\n%s", body)
	}
	token := tm[1]

	// Without a bridge running, saving bridge://llm with probe reports it plainly.
	_, body = post(t, c, ts.URL+"/settings", url.Values{"base_url": {"bridge://llm"}, "model": {"llama3.1"}, "probe": {"1"}}, false)
	if !strings.Contains(body, "not connected") {
		t.Fatalf("expected a 'bridge not connected' message:\n%s", body)
	}

	// Start the bridge on "the player's machine".
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go (&bridge.Client{Server: ts.URL, Token: token, Services: map[string]string{"llm": local.URL}, Log: log.New(io.Discard, "", 0)}).Run(ctx)
	srv := ts.Config.Handler.(*Server)
	deadline := time.Now().Add(5 * time.Second)
	for {
		p, _ := srv.Auth.Store.GetPlayerByEmail(context.Background(), "gpu@example.com")
		if srv.Hub.Connected(p.ID) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bridge never connected")
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, body = get(t, c, ts.URL+"/settings")
	if !strings.Contains(body, "<b>connected</b>") {
		t.Fatal("settings page does not show the bridge as connected")
	}

	// Now probe succeeds through the bridge.
	_, body = post(t, c, ts.URL+"/settings", url.Values{"base_url": {"bridge://llm"}, "model": {"llama3.1"}, "probe": {"1"}}, false)
	if !strings.Contains(body, "Endpoint answered") {
		t.Fatalf("probe through bridge failed:\n%s", body)
	}

	// Start a run and take a turn: classification and narration both go
	// server -> hub -> bridge -> local model.
	resp, err := c.PostForm(ts.URL+"/new", url.Values{"job": {"x"}, "idea": {"a diet app"}, "technical": {"yes"}, "age": {"25"}, "savings": {"100000"}, "time_scale_ms": {"3600000"}})
	if err != nil {
		t.Fatal(err)
	}
	runURL := resp.Request.URL.String()
	resp.Body.Close()
	code, body := post(t, c, runURL+"/act", url.Values{"action": {"build a landing page"}}, true)
	if code != 200 || !strings.Contains(body, "Narrated on your own GPU.") {
		t.Fatalf("turn through bridge failed: %d\n%s", code, body)
	}
	if hits < 3 { // probe + classify + turn
		t.Fatalf("local model hit %d times, expected >= 3", hits)
	}
	rec, _ := g.Store.GetRun(context.Background(), strings.TrimPrefix(runURL, ts.URL+"/run/"))
	if rec.Run.Player.Segment != "b2c" {
		t.Fatalf("classification did not come from the local model: %s", rec.Run.Player.Segment)
	}

	// Revoke the token: the bridge's next poll is rejected and turns fail cleanly.
	post(t, c, ts.URL+"/settings/bridge-token", url.Values{"revoke": {"1"}}, false)
	srv.Hub.StaleAfter = time.Millisecond
	time.Sleep(5 * time.Millisecond)
	if code, body := post(t, c, runURL+"/act", url.Values{"action": {"anything"}}, true); code != 400 || !strings.Contains(body, "not connected") {
		t.Fatalf("expected a clean 'bridge not connected' failure, got %d\n%s", code, body)
	}
}
