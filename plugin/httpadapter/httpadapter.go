// Package httpadapter carries the plugin.Plugin interface over HTTP+JSON.
//
// Wire protocol (v1). All bodies are JSON. Errors are a JSON {"error": "..."}
// with a 4xx/5xx status.
//
//	GET  /v1/manifest        -> plugin.Manifest
//	POST /v1/hook            plugin.HookRequest      -> plugin.HookResponse
//	GET  /v1/tools           -> []plugin.ToolSpec
//	POST /v1/invoke          plugin.InvokeRequest    -> plugin.InvokeResponse
//	GET  /v1/content         -> kb.Bundle
//	GET  /v1/llm-tools       -> []plugin.LLMToolSpec
//	POST /v1/llm-tool        plugin.LLMToolRequest   -> plugin.LLMToolResponse
//
// Server exposes any Plugin at those routes. Client implements Plugin by
// calling them. A built-in plugin wrapped in Server and consumed through
// Client must pass the same conformance tests as the built-in itself.
package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ramank775/founder-sim/kb"
	"github.com/ramank775/founder-sim/plugin"
)

// MaxBodyBytes bounds request and response bodies in both directions.
const MaxBodyBytes = 1 << 20

// ---------- Server ----------

// Server serves a Plugin over HTTP. Mount it at "/" (routes include /v1/).
type Server struct {
	P   plugin.Plugin
	mux *http.ServeMux
}

func NewServer(p plugin.Plugin) *Server {
	s := &Server{P: p, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /v1/manifest", func(w http.ResponseWriter, r *http.Request) {
		m, err := p.Manifest(r.Context())
		reply(w, m, err)
	})
	s.mux.HandleFunc("POST /v1/hook", func(w http.ResponseWriter, r *http.Request) {
		var req plugin.HookRequest
		if !decode(w, r, &req) {
			return
		}
		res, err := p.OnHook(r.Context(), req)
		reply(w, res, err)
	})
	s.mux.HandleFunc("GET /v1/tools", func(w http.ResponseWriter, r *http.Request) {
		t, err := p.Tools(r.Context())
		if t == nil {
			t = []plugin.ToolSpec{}
		}
		reply(w, t, err)
	})
	s.mux.HandleFunc("POST /v1/invoke", func(w http.ResponseWriter, r *http.Request) {
		var req plugin.InvokeRequest
		if !decode(w, r, &req) {
			return
		}
		res, err := p.Invoke(r.Context(), req)
		reply(w, res, err)
	})
	s.mux.HandleFunc("GET /v1/content", func(w http.ResponseWriter, r *http.Request) {
		c, err := p.Content(r.Context())
		reply(w, c, err)
	})
	s.mux.HandleFunc("GET /v1/llm-tools", func(w http.ResponseWriter, r *http.Request) {
		t, err := p.LLMTools(r.Context())
		if t == nil {
			t = []plugin.LLMToolSpec{}
		}
		reply(w, t, err)
	})
	s.mux.HandleFunc("POST /v1/llm-tool", func(w http.ResponseWriter, r *http.Request) {
		var req plugin.LLMToolRequest
		if !decode(w, r, &req) {
			return
		}
		res, err := p.CallLLMTool(r.Context(), req)
		reply(w, res, err)
	})
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	body := http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	if err := json.NewDecoder(body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return false
	}
	return true
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, plugin.ErrNoSuchTool) {
			status = http.StatusNotFound
		}
		writeErr(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---------- Client ----------

// Client is a plugin.Plugin backed by a remote HTTP plugin.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Token, if set, is sent as a bearer token so a remote plugin can
	// refuse strangers. Optional in v1.
	Token string
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: plugin.CallTimeout},
	}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("plugin %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if resp.StatusCode == http.StatusNotFound && strings.Contains(e.Error, plugin.ErrNoSuchTool.Error()) {
			return plugin.ErrNoSuchTool
		}
		return fmt.Errorf("plugin %s: %d %s", path, resp.StatusCode, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) Manifest(ctx context.Context) (m plugin.Manifest, err error) {
	err = c.do(ctx, http.MethodGet, "/v1/manifest", nil, &m)
	return
}
func (c *Client) OnHook(ctx context.Context, req plugin.HookRequest) (res plugin.HookResponse, err error) {
	err = c.do(ctx, http.MethodPost, "/v1/hook", req, &res)
	return
}
func (c *Client) Tools(ctx context.Context) (t []plugin.ToolSpec, err error) {
	err = c.do(ctx, http.MethodGet, "/v1/tools", nil, &t)
	return
}
func (c *Client) Invoke(ctx context.Context, req plugin.InvokeRequest) (res plugin.InvokeResponse, err error) {
	err = c.do(ctx, http.MethodPost, "/v1/invoke", req, &res)
	return
}
func (c *Client) Content(ctx context.Context) (p kb.Bundle, err error) {
	err = c.do(ctx, http.MethodGet, "/v1/content", nil, &p)
	return
}
func (c *Client) LLMTools(ctx context.Context) (t []plugin.LLMToolSpec, err error) {
	err = c.do(ctx, http.MethodGet, "/v1/llm-tools", nil, &t)
	return
}
func (c *Client) CallLLMTool(ctx context.Context, req plugin.LLMToolRequest) (res plugin.LLMToolResponse, err error) {
	err = c.do(ctx, http.MethodPost, "/v1/llm-tool", req, &res)
	return
}

// ListenAndServe runs a plugin as a standalone HTTP service. This is what a
// remote plugin's main() calls.
func ListenAndServe(addr string, p plugin.Plugin) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           NewServer(p),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

var _ plugin.Plugin = (*Client)(nil)
