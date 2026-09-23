package llm

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// Fake is a deterministic stand-in: it lets the whole loop run with no
// model at all (FOUNDER_SIM_FAKE_LLM=1) and drives tests. It answers
// classification and turns with plausible, valid JSON, and echoes the
// action so you can see the plumbing work.
type Fake struct {
	mu    sync.Mutex
	Calls []Request
	// Script, if non-empty, is returned in order instead of the defaults.
	Script []string
}

func (f *Fake) Complete(_ context.Context, req Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, req)
	if len(f.Script) > 0 {
		s := f.Script[0]
		f.Script = f.Script[1:]
		return Response{Content: s, Model: "fake"}, nil
	}
	last := req.Messages[len(req.Messages)-1].Content
	switch {
	case strings.Contains(last, `"segment"`):
		return Response{Content: `{"segment":"b2b_saas","reason":"fake"}`, Model: "fake"}, nil
	default:
		action := last
		if i := strings.LastIndex(last, "Founder's action:"); i >= 0 {
			action = strings.TrimSpace(last[i+len("Founder's action:"):])
		}
		out := map[string]any{
			"narration": "You spend part of the day on: " + strings.TrimSpace(action) + ". (fake model)",
			"slot_cost": 1,
			"outcomes": []map[string]any{
				{"weight": 0.6, "narration": "It goes roughly as planned."},
				{"weight": 0.4, "narration": "It takes longer than you thought and someone calls mid-way.",
					"deltas": []map[string]any{}},
			},
			"todos":   []string{"Follow up on: " + strings.TrimSpace(action)},
			"pending": []map[string]any{{"in_days": 2, "description": "Someone gets back to you about: " + strings.TrimSpace(action), "probability": 0.7}},
		}
		raw, _ := json.Marshal(out)
		return Response{Content: string(raw), Model: "fake"}, nil
	}
}
