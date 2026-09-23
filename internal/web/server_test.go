package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/ramank775/founder-sim/bridge"
	"github.com/ramank775/founder-sim/internal/auth"
	"github.com/ramank775/founder-sim/internal/game"
	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/internal/store"
	"github.com/ramank775/founder-sim/kb"
	"github.com/ramank775/founder-sim/plugin"
	"github.com/ramank775/founder-sim/plugins/notepad"
)

func newTestServer(t *testing.T) (*httptest.Server, *game.Service) {
	t.Helper()
	st, err := store.OpenFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base, err := kb.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	reg := plugin.NewRegistry()
	if _, err := reg.Register(context.Background(), notepad.New()); err != nil {
		t.Fatal(err)
	}
	cipher, _ := auth.NewCipher("test-master-key-1234567890")
	ts := httptest.NewServer(http.NotFoundHandler())
	a := &auth.Auth{Store: st, Sender: auth.LogSender{}, BaseURL: ts.URL, DevEcho: true}
	now := int64(1_000_000)
	g := &game.Service{Store: st, KB: base, Plugins: reg,
		NewClient: func(llm.Config) llm.Client { return &llm.Fake{} },
		Now:       func() int64 { return now },
	}
	srv, err := New(a, g, cipher, bridge.NewHub(), true)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = srv
	return ts, g
}

func client(t *testing.T) *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func post(t *testing.T, c *http.Client, u string, form url.Values, hx bool) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func get(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWholeLoop(t *testing.T) {
	ts, g := newTestServer(t)
	defer ts.Close()
	c := client(t)

	// Login via magic link (dev echo puts it in the page).
	_, body := post(t, c, ts.URL+"/login", url.Values{"email": {"raman@example.com"}}, false)
	m := regexp.MustCompile(`href="(` + regexp.QuoteMeta(ts.URL) + `/login/[a-f0-9]+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no magic link in page:\n%s", body)
	}
	if code, body := get(t, c, m[1]); code != 200 || !strings.Contains(body, "Runs") {
		t.Fatalf("login did not land on home: %d\n%s", code, body)
	}
	// Second use of the same link must fail.
	if _, body := get(t, c, m[1]); !strings.Contains(body, "invalid or has expired") {
		t.Fatal("magic link was reusable")
	}

	// Configure a model (fake in tests; the config is still stored encrypted).
	post(t, c, ts.URL+"/settings", url.Values{"base_url": {"http://fake"}, "model": {"fake"}, "api_key": {"sk-secret"}}, false)

	// Start a run with the notepad enabled and a 10-second day.
	resp, err := c.PostForm(ts.URL+"/new", url.Values{
		"job": {"backend dev"}, "idea": {"GST invoices over WhatsApp"}, "technical": {"yes"},
		"age": {"30"}, "marital": {"married"}, "kids": {"none"}, "savings": {"6,00,000"},
		"ob_label_1": {"rent"}, "ob_amount_1": {"30000"},
		"time_scale_ms": {"10000"}, "plugins": {"notepad"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runURL := resp.Request.URL.String()
	resp.Body.Close()
	if !strings.Contains(runURL, "/run/") {
		t.Fatalf("expected redirect to run, got %s", runURL)
	}

	// Take an action.
	code, body := post(t, c, runURL+"/act", url.Values{"action": {"Tell my father about it over dinner"}}, true)
	if code != 200 || !strings.Contains(body, "Tell my father") || !strings.Contains(body, "raw model output") {
		t.Fatalf("act failed: %d\n%s", code, body)
	}
	if !strings.Contains(body, "slots left <b>3</b>") {
		t.Fatalf("slot not consumed:\n%s", body)
	}

	// Money hidden until pinned; checking costs a slot.
	if strings.Contains(body, "money <b>") {
		t.Fatal("money shown without pin")
	}
	_, body = post(t, c, runURL+"/balance", nil, true)
	if !strings.Contains(body, "₹6,00,000") || !strings.Contains(body, "slots left <b>2</b>") {
		t.Fatalf("balance check wrong:\n%s", body)
	}
	_, body = post(t, c, runURL+"/pin", nil, true)
	if !strings.Contains(body, "money <b>₹6,00,000</b>") {
		t.Fatalf("pin failed:\n%s", body)
	}

	// Notepad tool: add a note, set a reminder.
	_, body = post(t, c, runURL+"/tool/notepad/notepad", url.Values{"input": {"call the CA"}}, true)
	if !strings.Contains(body, "call the CA") {
		t.Fatalf("notepad add failed:\n%s", body)
	}
	_, body = post(t, c, runURL+"/tool/notepad/notepad", url.Values{"input": {"remind 1: chase the agency"}}, true)
	if !strings.Contains(body, "Reminder set for day 1") {
		t.Fatalf("reminder failed:\n%s", body)
	}

	// Advance the clock two days: reminder fires, notepad metric appears.
	g.Now = func() int64 { return 1_000_000 + 2*10_000 }
	_, body = get(t, c, runURL)
	if !strings.Contains(body, "Reminder you set: chase the agency") {
		t.Fatalf("reminder did not fire:\n%s", body)
	}
	if !strings.Contains(body, "Day <b>2</b>") || !strings.Contains(body, "slots left <b>4</b>") {
		t.Fatalf("clock did not advance / slots not reset:\n%s", body)
	}

	// Ignore the todo; the engine remembers.
	m = regexp.MustCompile(`/todo/(todo-[^/"]+)/dismiss`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no todo to dismiss:\n%s", body)
	}
	post(t, c, runURL+"/todo/"+m[1]+"/dismiss", nil, true)
	rec, _ := g.Store.GetRun(context.Background(), strings.TrimPrefix(runURL, ts.URL+"/run/"))
	if len(rec.Run.Missed) == 0 {
		t.Fatal("dismissed todo not recorded as missed")
	}
	// Learned items persisted on the player: nothing discovered yet in 2 days, fine either way.
	if rec.Run.Player.Segment != "b2b_saas" || len(rec.Run.Risks) < 10 {
		t.Fatalf("run not set up: segment=%s risks=%d", rec.Run.Player.Segment, len(rec.Run.Risks))
	}

	// Exit; only the founder can.
	resp, _ = c.PostForm(runURL+"/exit", url.Values{"kind": {"walked_away"}, "note": {"not yet"}})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "Run over") || !strings.Contains(string(b), "walked_away on day 2") {
		t.Fatalf("exit page wrong:\n%s", b)
	}
	if code, _ := post(t, c, runURL+"/act", url.Values{"action": {"anything"}}, true); code != 400 {
		t.Fatalf("exited run still accepts actions: %d", code)
	}

	// A different user cannot see this run.
	c2 := client(t)
	_, body = post(t, c2, ts.URL+"/login", url.Values{"email": {"other@example.com"}}, false)
	m = regexp.MustCompile(`href="(` + regexp.QuoteMeta(ts.URL) + `/login/[a-f0-9]+)"`).FindStringSubmatch(body)
	get(t, c2, m[1])
	if code, _ := get(t, c2, runURL); code != 404 {
		t.Fatalf("run leaked to another player: %d", code)
	}
}

func TestSanitize(t *testing.T) {
	in := `<div class="x" onclick="evil()"><script>alert(1)</script><b>ok</b><a href="x">l</a></div>`
	out := sanitizeHTML(in)
	if strings.Contains(out, "<script") || strings.Contains(out, "onclick") || strings.Contains(out, "<a ") {
		t.Fatalf("sanitizer let something through: %s", out)
	}
	if !strings.Contains(out, `<div class="x">`) || !strings.Contains(out, "<b>ok</b>") {
		t.Fatalf("sanitizer stripped allowed markup: %s", out)
	}
}

func TestIndianCommas(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 100000: "1,00,000", 12345678: "1,23,45,678", -50000: "-50,000"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %s, want %s", n, got, want)
		}
	}
}
