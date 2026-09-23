package bridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNoBridge means no bridge has polled for this player recently.
var ErrNoBridge = errors.New("your bridge is not connected (run founder-sim-bridge on your machine)")

// Hub is the server side. One per process. It queues requests per player
// and hands them to whichever bridge polls with that player's token.
type Hub struct {
	mu       sync.Mutex
	queues   map[string]chan *pending // playerID -> queue
	inflight map[string]*pending      // request id -> waiting caller
	lastSeen map[string]time.Time     // playerID -> last poll
	// StaleAfter: a player whose bridge has not polled within this window
	// is treated as disconnected and calls fail fast instead of queueing.
	StaleAfter time.Duration
	// CallTimeout bounds one round trip through the bridge.
	CallTimeout time.Duration
}

type pending struct {
	playerID string
	req      Request
	done     chan Result
}

func NewHub() *Hub {
	return &Hub{
		queues:      map[string]chan *pending{},
		inflight:    map[string]*pending{},
		lastSeen:    map[string]time.Time{},
		StaleAfter:  2 * PollWindowSeconds * time.Second,
		CallTimeout: 180 * time.Second,
	}
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Hub) queue(playerID string) chan *pending {
	h.mu.Lock()
	defer h.mu.Unlock()
	q, ok := h.queues[playerID]
	if !ok {
		q = make(chan *pending, 16)
		h.queues[playerID] = q
	}
	return q
}

// Connected reports whether a bridge for the player polled recently.
func (h *Hub) Connected(playerID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.lastSeen[playerID]
	return ok && time.Since(t) < h.StaleAfter
}

// Poll is called by the HTTP handler on behalf of a bridge. It blocks up to
// PollWindowSeconds for a request. ok=false means nothing to do.
func (h *Hub) Poll(ctx context.Context, playerID string) (Request, bool) {
	h.mu.Lock()
	h.lastSeen[playerID] = time.Now()
	h.mu.Unlock()
	q := h.queue(playerID)
	t := time.NewTimer(PollWindowSeconds * time.Second)
	defer t.Stop()
	select {
	case p := <-q:
		return p.req, true
	case <-t.C:
		return Request{}, false
	case <-ctx.Done():
		return Request{}, false
	}
}

// Submit delivers a bridge's result to the waiting caller.
func (h *Hub) Submit(playerID, id string, res Result) error {
	h.mu.Lock()
	p, ok := h.inflight[id]
	if ok && p.playerID == playerID {
		delete(h.inflight, id)
	}
	h.mu.Unlock()
	if !ok || p.playerID != playerID {
		return fmt.Errorf("unknown request %s", id)
	}
	select {
	case p.done <- res:
	default:
	}
	return nil
}

// Do sends one request through the player's bridge and waits for the result.
func (h *Hub) Do(ctx context.Context, playerID string, req Request) (Result, error) {
	if !h.Connected(playerID) {
		return Result{}, ErrNoBridge
	}
	if req.ID == "" {
		req.ID = newID()
	}
	if req.Service == "" {
		return Result{}, errors.New("bridge request has no service name")
	}
	p := &pending{playerID: playerID, req: req, done: make(chan Result, 1)}
	h.mu.Lock()
	h.inflight[req.ID] = p
	h.mu.Unlock()
	cleanup := func() {
		h.mu.Lock()
		delete(h.inflight, req.ID)
		h.mu.Unlock()
	}
	ctx, cancel := context.WithTimeout(ctx, h.CallTimeout)
	defer cancel()
	select {
	case h.queue(playerID) <- p:
	case <-ctx.Done():
		cleanup()
		return Result{}, fmt.Errorf("bridge queue full or timed out: %w", ctx.Err())
	}
	select {
	case res := <-p.done:
		if res.Error != "" {
			return res, errors.New("bridge: " + res.Error)
		}
		return res, nil
	case <-ctx.Done():
		cleanup()
		return Result{}, fmt.Errorf("bridge: no response from service %q on your machine: %w", req.Service, ctx.Err())
	}
}

// Transport is an http.RoundTripper that sends every request through the
// player's bridge to one named local service. Any component that takes an
// *http.Client can be pointed at a player's machine this way; the host in
// the URL is ignored, only method, path, query and body are forwarded.
type Transport struct {
	Hub      *Hub
	PlayerID string
	Service  string
}

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		body = b
	}
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	res, err := t.Hub.Do(r.Context(), t.PlayerID, Request{
		Service: t.Service, Method: r.Method, Path: path, ContentType: r.Header.Get("Content-Type"), Body: body,
	})
	if err != nil {
		return nil, err
	}
	resp := &http.Response{
		StatusCode: res.Status,
		Status:     fmt.Sprintf("%d %s", res.Status, http.StatusText(res.Status)),
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(res.Body)),
		Request:    r,
	}
	if res.ContentType != "" {
		resp.Header.Set("Content-Type", res.ContentType)
	}
	return resp, nil
}

// ServiceFromURL parses "bridge://<service>[/path]" and returns the service
// name. ok is false for any other URL.
func ServiceFromURL(baseURL string) (service string, ok bool) {
	u := strings.TrimSpace(baseURL)
	if !strings.HasPrefix(strings.ToLower(u), Scheme+"://") {
		return "", false
	}
	rest := u[len(Scheme)+3:]
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.ToLower(strings.TrimSpace(rest))
	if rest == "" {
		return "", false
	}
	return rest, true
}

// HTTPClient returns an *http.Client that reaches the named service on the
// player's machine through the hub.
func (h *Hub) HTTPClient(playerID, service string) *http.Client {
	return &http.Client{Transport: &Transport{Hub: h, PlayerID: playerID, Service: service}, Timeout: h.CallTimeout}
}

// NewToken makes a bridge token for a player.
func NewToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
