// Package llm talks to any OpenAI-compatible chat completions endpoint
// (LiteLLM, Ollama, vLLM, OpenRouter, OpenAI itself) and turns the model's
// output into typed engine proposals. The model narrates and proposes; it
// never rolls dice and never touches state.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config is a player's model configuration. Stored per player (key encrypted).
type Config struct {
	BaseURL  string `json:"base_url"` // e.g. https://api.openai.com/v1, http://localhost:11434/v1
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
	JSONMode bool   `json:"json_mode"` // endpoint honours response_format json_object
	// HTTP, if set, is used instead of a default client. This is how a
	// "bridge://<service>" base URL is routed through the player's bridge:
	// the caller supplies a client whose transport is the bridge.
	HTTP *http.Client `json:"-"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Messages    []Message
	JSON        bool // ask for a JSON object
	MaxTokens   int
	Temperature float64
}

type Response struct {
	Content string
	Model   string
	Usage   struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
	}
}

// Client is the one seam. OpenAI-compatible HTTP is the real one; Fake is
// for tests and for playing without a model.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// ---------- OpenAI-compatible ----------

type OpenAICompat struct {
	Cfg  Config
	HTTP *http.Client
}

func NewOpenAICompat(cfg Config) *OpenAICompat {
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 120 * time.Second}
	}
	return &OpenAICompat{Cfg: cfg, HTTP: hc}
}

type oaReq struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	MaxTokens      int       `json:"max_tokens,omitempty"`
	Temperature    float64   `json:"temperature"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

type oaResp struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompat) Complete(ctx context.Context, req Request) (Response, error) {
	body := oaReq{Model: c.Cfg.Model, Messages: req.Messages, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	if req.JSON && c.Cfg.JSONMode {
		body.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}
	raw, _ := json.Marshal(body)
	url := strings.TrimRight(c.Cfg.BaseURL, "/") + "/chat/completions"
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}
	hr.Header.Set("Content-Type", "application/json")
	if c.Cfg.APIKey != "" {
		hr.Header.Set("Authorization", "Bearer "+c.Cfg.APIKey)
	}
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		return Response{}, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var out oaResp
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("llm: non-JSON response (%d): %s", resp.StatusCode, truncate(string(data), 200))
	}
	if resp.StatusCode >= 400 {
		msg := truncate(string(data), 200)
		if out.Error != nil {
			msg = out.Error.Message
		}
		return Response{}, fmt.Errorf("llm: %d %s", resp.StatusCode, msg)
	}
	if len(out.Choices) == 0 {
		return Response{}, errors.New("llm: no choices")
	}
	r := Response{Content: out.Choices[0].Message.Content, Model: out.Model}
	r.Usage.Prompt, r.Usage.Completion = out.Usage.Prompt, out.Usage.Completion
	return r, nil
}

// Probe checks the endpoint answers and whether it honours JSON mode.
// Returns a corrected Config.
func Probe(ctx context.Context, cfg Config) (Config, error) {
	c := NewOpenAICompat(cfg)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cfg.JSONMode = true
	c.Cfg = cfg
	_, err := c.Complete(ctx, Request{JSON: true, MaxTokens: 20, Messages: []Message{{Role: "user", Content: `Reply with the JSON object {"ok":true}`}}})
	if err == nil {
		return cfg, nil
	}
	cfg.JSONMode = false
	c.Cfg = cfg
	if _, err2 := c.Complete(ctx, Request{MaxTokens: 20, Messages: []Message{{Role: "user", Content: "Say ok."}}}); err2 != nil {
		return cfg, err2
	}
	return cfg, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ExtractJSON pulls the first JSON object out of model text that may be
// wrapped in prose or a ```json fence. Local models do this constantly.
func ExtractJSON(s string) (string, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			s = rest[:j]
		} else {
			s = rest
		}
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return "", errors.New("no JSON object in model output")
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case ch == '\\':
				esc = true
			case ch == '"':
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", errors.New("unterminated JSON object in model output")
}

// ConfigFromEnv reads FOUNDER_SIM_LLM_BASE_URL, FOUNDER_SIM_LLM_MODEL,
// FOUNDER_SIM_LLM_API_KEY (and FOUNDER_SIM_LLM_JSON_MODE=1) for tools that
// run outside a player session, such as content ingestion.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:  os.Getenv("FOUNDER_SIM_LLM_BASE_URL"),
		Model:    os.Getenv("FOUNDER_SIM_LLM_MODEL"),
		APIKey:   os.Getenv("FOUNDER_SIM_LLM_API_KEY"),
		JSONMode: os.Getenv("FOUNDER_SIM_LLM_JSON_MODE") == "1",
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return cfg, errors.New("set FOUNDER_SIM_LLM_BASE_URL and FOUNDER_SIM_LLM_MODEL (and FOUNDER_SIM_LLM_API_KEY if needed)")
	}
	return cfg, nil
}
