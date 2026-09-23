package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client is the bridge side: runs on the player's machine, dials out to
// the game server, forwards requests to named local HTTP services.
type Client struct {
	Server string // game server base URL, e.g. https://sim.example.com
	Token  string // bridge token from Settings
	// Services maps a service name to its local base URL, e.g.
	// "llm" -> "http://localhost:11434/v1". Requests for names not listed
	// here are refused; the server never learns any other URL.
	Services map[string]string
	HTTP     *http.Client
	Log      *log.Logger
	// MaxConcurrent bounds simultaneous local calls (a home GPU is not a cluster).
	MaxConcurrent int
}

func (c *Client) init() {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 200 * time.Second}
	}
	if c.Log == nil {
		c.Log = log.Default()
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 2
	}
	c.Server = strings.TrimRight(c.Server, "/")
	for k, v := range c.Services {
		c.Services[k] = strings.TrimRight(v, "/")
	}
}

// Run polls until ctx is cancelled. Transient errors back off and retry.
func (c *Client) Run(ctx context.Context) error {
	c.init()
	sem := make(chan struct{}, c.MaxConcurrent)
	backoff := time.Second
	for {
		req, ok, err := c.poll(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			c.Log.Printf("poll: %v (retrying in %s)", err, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		if !ok {
			continue
		}
		sem <- struct{}{}
		go func(r Request) {
			defer func() { <-sem }()
			res := c.forward(ctx, r)
			if err := c.submit(ctx, r.ID, res); err != nil {
				c.Log.Printf("submit %s: %v", r.ID, err)
			}
		}(req)
	}
}

func (c *Client) poll(ctx context.Context) (Request, bool, error) {
	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Server+"/bridge/poll", nil)
	if err != nil {
		return Request{}, false, err
	}
	hr.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return Request{}, false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return Request{}, false, nil
	case http.StatusOK:
		var r Request
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&r); err != nil {
			return Request{}, false, err
		}
		return r, true, nil
	case http.StatusUnauthorized:
		return Request{}, false, fmt.Errorf("server rejected the bridge token; generate a new one in Settings")
	default:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return Request{}, false, fmt.Errorf("server said %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func (c *Client) forward(ctx context.Context, r Request) Result {
	base, ok := c.Services[strings.ToLower(r.Service)]
	if !ok {
		c.Log.Printf("refused request for unknown service %q", r.Service)
		return Result{Error: fmt.Sprintf("this bridge exposes no service named %q", r.Service)}
	}
	method := r.Method
	if method == "" {
		method = http.MethodPost
	}
	path := r.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	hr, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(r.Body))
	if err != nil {
		return Result{Error: err.Error()}
	}
	if r.ContentType != "" {
		hr.Header.Set("Content-Type", r.ContentType)
	}
	start := time.Now()
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		c.Log.Printf("%s: %s %s: %v", r.Service, method, path, err)
		return Result{Error: r.Service + " unreachable: " + err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Result{Error: err.Error()}
	}
	c.Log.Printf("%s: %s %s -> %d (%d bytes, %s)", r.Service, method, path, resp.StatusCode, len(body), time.Since(start).Round(time.Millisecond))
	return Result{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}
}

func (c *Client) submit(ctx context.Context, id string, res Result) error {
	raw, _ := json.Marshal(res)
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Server+"/bridge/result/"+id, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	hr.Header.Set("Authorization", "Bearer "+c.Token)
	hr.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server said %d", resp.StatusCode)
	}
	return nil
}
