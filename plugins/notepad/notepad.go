// Package notepad is the reference plugin: a founder's notepad with
// reminders. It is compiled into the binary and also runs standalone over
// HTTP (see cmd/notepad-plugin), and it exercises three of the four
// capabilities: a player-facing tool, a hook, and an LLM-visible tool.
//
// Using the notepad is the player's choice. The engine tracks what they
// missed regardless; this only tracks what they chose to write down.
package notepad

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/ramank775/founder-sim/engine"
	"github.com/ramank775/founder-sim/plugin"
)

type Notepad struct{ plugin.Base }

type note struct {
	ID   int    `json:"id"`
	Day  int    `json:"day"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type state struct {
	Notes  []note `json:"notes"`
	NextID int    `json:"next_id"`
}

func New() *Notepad { return &Notepad{} }

func (n *Notepad) Manifest(context.Context) (plugin.Manifest, error) {
	return plugin.Manifest{
		Name:            "notepad",
		Version:         "0.1.0",
		ProtocolVersion: plugin.ProtocolVersion,
		Description:     "A notepad with reminders. Write things down, or don't.",
		Capabilities:    []string{plugin.CapTools, plugin.CapHooks, plugin.CapLLMTools},
		Hooks:           []string{plugin.HookDayAdvanced},
	}, nil
}

func (n *Notepad) Tools(context.Context) ([]plugin.ToolSpec, error) {
	return []plugin.ToolSpec{{
		Name:  "notepad",
		Title: "Notepad",
		Description: "Type a note to add it. `done 3` ticks note 3. `remind 5: call the CA` " +
			"sets a reminder that surfaces in 5 days. Empty input shows the pad.",
		Panel: true,
	}}, nil
}

func loadState(raw json.RawMessage) state {
	var s state
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	if s.NextID == 0 {
		s.NextID = 1
	}
	return s
}

func saveState(s state) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

func (n *Notepad) Invoke(_ context.Context, req plugin.InvokeRequest) (plugin.InvokeResponse, error) {
	if req.Tool != "notepad" {
		return plugin.InvokeResponse{}, plugin.ErrNoSuchTool
	}
	s := loadState(req.State)
	in := strings.TrimSpace(req.Input)
	var prop *engine.Proposal
	var msg string

	switch {
	case in == "":
		// just render
	case strings.HasPrefix(strings.ToLower(in), "done "):
		id, err := strconv.Atoi(strings.TrimSpace(in[5:]))
		if err != nil {
			msg = "done needs a note number"
			break
		}
		found := false
		for i := range s.Notes {
			if s.Notes[i].ID == id {
				s.Notes[i].Done = true
				found = true
			}
		}
		if !found {
			msg = fmt.Sprintf("no note %d", id)
		}
	case strings.HasPrefix(strings.ToLower(in), "remind "):
		rest := strings.TrimSpace(in[7:])
		days, text, ok := strings.Cut(rest, ":")
		d, err := strconv.Atoi(strings.TrimSpace(days))
		if !ok || err != nil || d < 1 || d > 365 || strings.TrimSpace(text) == "" {
			msg = "use: remind <days>: <text>"
			break
		}
		text = strings.TrimSpace(text)
		s.Notes = append(s.Notes, note{ID: s.NextID, Day: req.View.Day, Text: fmt.Sprintf("(reminder, day %d) %s", req.View.Day+d, text)})
		s.NextID++
		// The reminder is a pending event. The engine owns when it fires;
		// probability 1 means it will, but only when that day actually comes.
		prop = &engine.Proposal{Pending: []engine.PendingProposal{{
			InDays: d, Description: "Reminder you set: " + text, Probability: 1,
		}}}
		msg = fmt.Sprintf("Reminder set for day %d.", req.View.Day+d)
	default:
		s.Notes = append(s.Notes, note{ID: s.NextID, Day: req.View.Day, Text: in})
		s.NextID++
	}

	return plugin.InvokeResponse{
		Text:     msg,
		HTML:     render(s, msg),
		Proposal: prop,
		State:    saveState(s),
	}, nil
}

func render(s state, msg string) string {
	var b strings.Builder
	b.WriteString("<div class=\"notepad\">")
	if msg != "" {
		b.WriteString("<p class=\"muted\">" + html.EscapeString(msg) + "</p>")
	}
	open := 0
	b.WriteString("<ul>")
	for _, nt := range s.Notes {
		if nt.Done {
			continue
		}
		open++
		fmt.Fprintf(&b, "<li><b>%d.</b> %s <span class=\"muted\">(day %d)</span></li>", nt.ID, html.EscapeString(nt.Text), nt.Day)
	}
	b.WriteString("</ul>")
	if open == 0 {
		b.WriteString("<p class=\"muted\">Nothing written down.</p>")
	}
	b.WriteString("</div>")
	return b.String()
}

// OnHook: on a new day, nothing changes in the pad, but we report how many
// open notes exist as a namespaced metric so the player can pin it.
func (n *Notepad) OnHook(_ context.Context, req plugin.HookRequest) (plugin.HookResponse, error) {
	if req.Kind != plugin.HookDayAdvanced {
		return plugin.HookResponse{}, nil
	}
	s := loadState(req.State)
	open := 0
	for _, nt := range s.Notes {
		if !nt.Done {
			open++
		}
	}
	return plugin.HookResponse{Proposal: &engine.Proposal{
		Deltas: []engine.Delta{{Op: "metric_set", Key: "notepad.open_notes", Amount: int64(open)}},
	}}, nil
}

// LLMTools lets the narrator peek at the pad, so an NPC can say "you wrote
// down 'call the CA' four days ago" — if the player chose to write it.
func (n *Notepad) LLMTools(context.Context) ([]plugin.LLMToolSpec, error) {
	return []plugin.LLMToolSpec{{
		Name:        "notepad_read",
		Description: "Read the founder's own notepad (open notes only). Returns plain text.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}}, nil
}

func (n *Notepad) CallLLMTool(_ context.Context, req plugin.LLMToolRequest) (plugin.LLMToolResponse, error) {
	if req.Tool != "notepad_read" {
		return plugin.LLMToolResponse{}, plugin.ErrNoSuchTool
	}
	s := loadState(req.State)
	var lines []string
	for _, nt := range s.Notes {
		if !nt.Done {
			lines = append(lines, fmt.Sprintf("day %d: %s", nt.Day, nt.Text))
		}
	}
	if len(lines) == 0 {
		return plugin.LLMToolResponse{Result: "The notepad is empty."}, nil
	}
	return plugin.LLMToolResponse{Result: strings.Join(lines, "\n")}, nil
}

var _ plugin.Plugin = (*Notepad)(nil)
