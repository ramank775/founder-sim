package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type tokens map[string]string // token -> player

func (t tokens) PlayerIDForBridgeToken(_ context.Context, tok string) (string, error) {
	if p, ok := t[tok]; ok {
		return p, nil
	}
	return "", errors.New("nope")
}

// A fake "local model" that echoes which upstream it is and what it got.
func localService(name string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service": name, "path": r.URL.Path, "method": r.Method, "got": string(body),
		})
	}))
}

func TestTwoPlayersTwoBridgesIsolated(t *testing.T) {
	hub := NewHub()
	hub.CallTimeout = 10 * time.Second
	server := httptest.NewServer(Handler(hub, tokens{"tok-a": "alice", "tok-b": "bob"}))
	defer server.Close()

	upA := localService("alice-ollama")
	upB := localService("bob-vllm")
	defer upA.Close()
	defer upB.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	quiet := log.New(io.Discard, "", 0)
	go (&Client{Server: server.URL, Token: "tok-a", Services: map[string]string{"llm": upA.URL}, Log: quiet}).Run(ctx)
	go (&Client{Server: server.URL, Token: "tok-b", Services: map[string]string{"llm": upB.URL, "plugin": upB.URL}, Log: quiet}).Run(ctx)

	// Wait until both bridges have polled once.
	deadline := time.Now().Add(5 * time.Second)
	for !(hub.Connected("alice") && hub.Connected("bob")) {
		if time.Now().After(deadline) {
			t.Fatal("bridges never connected")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Fire requests for both players concurrently; each must land on its own upstream.
	var wg sync.WaitGroup
	call := func(player, service, want string) {
		defer wg.Done()
		c := hub.HTTPClient(player, service)
		resp, err := c.Post("http://ignored/chat/completions", "application/json", strings.NewReader(`{"q":"`+player+`"}`))
		if err != nil {
			t.Errorf("%s: %v", player, err)
			return
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if out["service"] != want || out["path"] != "/chat/completions" || !strings.Contains(out["got"].(string), player) {
			t.Errorf("%s got %v", player, out)
		}
	}
	for i := 0; i < 3; i++ {
		wg.Add(2)
		go call("alice", "llm", "alice-ollama")
		go call("bob", "llm", "bob-vllm")
	}
	wg.Wait()

	// A service the bridge does not expose is refused by the bridge, not the server.
	resp, err := hub.HTTPClient("alice", "plugin").Get("http://x/v1/manifest")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected refusal for unknown service")
	}
	if !strings.Contains(err.Error(), `no service named "plugin"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	// Bob does expose "plugin".
	resp, err = hub.HTTPClient("bob", "plugin").Get("http://x/v1/manifest")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// A player with no bridge fails fast.
	if _, err := hub.HTTPClient("carol", "llm").Get("http://x/"); err == nil || !errors.Is(errors.Unwrap(err), ErrNoBridge) && !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected ErrNoBridge, got %v", err)
	}

	// Bad token is 401.
	req, _ := http.NewRequest("GET", server.URL+"/bridge/poll", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	r, _ := http.DefaultClient.Do(req)
	if r.StatusCode != 401 {
		t.Fatalf("bad token got %d", r.StatusCode)
	}
	r.Body.Close()

	// Bob cannot submit a result for Alice's request id.
	if err := hub.Submit("bob", "no-such-id", Result{Status: 200}); err == nil {
		t.Fatal("submit for unknown id should fail")
	}
}

func TestServiceFromURL(t *testing.T) {
	cases := map[string]string{"bridge://llm": "llm", "BRIDGE://LLM/v1": "llm", "bridge://": "", "http://x": "", "bridge://notepad?x=1": "notepad"}
	for in, want := range cases {
		got, ok := ServiceFromURL(in)
		if got != want || ok != (want != "") {
			t.Errorf("ServiceFromURL(%q) = %q,%v", in, got, ok)
		}
	}
}
