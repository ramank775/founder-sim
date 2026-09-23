package engine

import (
	"fmt"
	"math"
)

// Time is faked in v1: no background workers. When the player returns, the
// engine computes how many in-game days elapsed from wall-clock time and
// resolves each day in order. The clock keeps running while they are away.

// DayAt returns the in-game day for a wall-clock time, from the current anchor.
func (r *Run) DayAt(nowMs int64) int {
	if r.TimeScaleMs <= 0 {
		return r.CurrentDay
	}
	anchorAt := r.AnchorAtMs
	if anchorAt == 0 {
		anchorAt = r.StartedAt
	}
	elapsed := nowMs - anchorAt
	if elapsed < 0 {
		return r.CurrentDay
	}
	return r.AnchorDay + int(elapsed/r.TimeScaleMs)
}

// SetTimeScale changes real-ms-per-day without jumping the current day.
func (r *Run) SetTimeScale(nowMs int64, scaleMs int64) error {
	if scaleMs < 1000 {
		return fmt.Errorf("time scale too small: %d ms/day", scaleMs)
	}
	r.CurrentDay = r.DayAt(nowMs)
	r.AnchorDay = r.CurrentDay
	r.AnchorAtMs = nowMs
	r.TimeScaleMs = scaleMs
	return nil
}

// DayReport is what happened during one resolved day while the player was away.
type DayReport struct {
	Day    int
	Fired  []PendingEvent
	Risks  []RiskItem // checklist items that triggered
	Missed []MissedThing
	Bills  int64 // obligations deducted
}

// CatchUp advances the run to the day implied by nowMs, resolving each
// intermediate day. It caps at MaxCatchUpDays so a run abandoned for a year
// does not resolve 8,000 days on one request; the remainder is dropped and
// the clock re-anchored. Returns one report per resolved day.
func (r *Run) CatchUp(nowMs int64) []DayReport {
	if r.Status != "active" {
		return nil
	}
	target := r.DayAt(nowMs)
	if target <= r.CurrentDay {
		return nil
	}
	if target-r.CurrentDay > MaxCatchUpDays {
		target = r.CurrentDay + MaxCatchUpDays
		// re-anchor so the dropped days do not keep coming back
		r.AnchorDay = target
		r.AnchorAtMs = nowMs
	}
	var reports []DayReport
	for r.CurrentDay < target {
		r.CurrentDay++
		reports = append(reports, r.resolveDay())
	}
	return reports
}

// MaxCatchUpDays bounds batch resolution on return.
const MaxCatchUpDays = 60

// resolveDay runs everything that happens at the start of a new day.
func (r *Run) resolveDay() DayReport {
	rep := DayReport{Day: r.CurrentDay}
	r.SlotsLeft = SlotsPerDay

	// 1. Monthly obligations never pause.
	if r.CurrentDay > 0 && r.CurrentDay%30 == 0 {
		for _, o := range r.Player.MonthlyObligations {
			r.World.Money -= o.Amount
			rep.Bills += o.Amount
		}
		if rep.Bills > 0 {
			r.Log = append(r.Log, LogEntry{Day: r.CurrentDay, Kind: "system",
				Text: fmt.Sprintf("Month-end. Obligations went out: ₹%d.", rep.Bills)})
		}
	}

	// 2. Pending events due today roll against their probability.
	for i := range r.Pending {
		ev := &r.Pending[i]
		if ev.Fired || ev.DueDay > r.CurrentDay {
			continue
		}
		ev.Fired = true // due events are consumed whether or not they fire
		if r.Roll(ev.Probability) {
			rep.Fired = append(rep.Fired, *ev)
			r.Log = append(r.Log, LogEntry{Day: r.CurrentDay, Kind: "event", Text: ev.Description, Source: ev.Source})
			r.Todos = append(r.Todos, Todo{
				ID: fmt.Sprintf("todo-%d-%s", r.CurrentDay, ev.ID), Day: r.CurrentDay,
				Text: ev.Description, Source: ev.Source,
			})
		}
	}

	// 3. Unticked risk items may surface. Weekly probability -> daily.
	for i := range r.Risks {
		ri := &r.Risks[i]
		if ri.Ticked || ri.TriggerProbPerWeek <= 0 || !r.riskApplies(ri) {
			continue
		}
		daily := 1 - math.Pow(1-clamp01(ri.TriggerProbPerWeek), 1.0/7.0)
		if r.Roll(daily) {
			ri.LastTriggeredDay = r.CurrentDay
			ri.Discovered = true
			rep.Risks = append(rep.Risks, *ri)
			// The consequence is best or worst case; the engine rolls 50/50 in v1.
			text := ri.BestCase
			if r.Roll(0.5) {
				text = ri.WorstCase
			}
			r.Log = append(r.Log, LogEntry{Day: r.CurrentDay, Kind: "event", Text: text, Source: "risk:" + ri.ID})
			r.Todos = append(r.Todos, Todo{
				ID: fmt.Sprintf("todo-%d-risk-%s", r.CurrentDay, ri.ID), Day: r.CurrentDay,
				Text: text, Source: "risk:" + ri.ID,
			})
		}
	}

	// 4. Things the player ignored resurface, each with a small daily chance.
	for i := range r.Missed {
		m := &r.Missed[i]
		if m.Resurfaced {
			continue
		}
		age := r.CurrentDay - m.Day
		if age < 3 {
			continue
		}
		if r.Roll(0.08) {
			m.Resurfaced = true
			rep.Missed = append(rep.Missed, *m)
			r.Log = append(r.Log, LogEntry{Day: r.CurrentDay, Kind: "event",
				Text: "Something you let slide comes back: " + m.What, Source: "missed"})
		}
	}

	// 5. Whatever was left undone yesterday becomes a missed thing.
	for i := range r.Todos {
		t := &r.Todos[i]
		if t.Done || t.Dismissed || t.Day >= r.CurrentDay-1 {
			continue
		}
		if r.CurrentDay-t.Day == 2 { // exactly once, two days after it came up
			r.Missed = append(r.Missed, MissedThing{Day: t.Day, What: t.Text})
		}
	}

	return rep
}

// riskApplies evaluates a risk item's AppliesWhen gate against world state.
func (r *Run) riskApplies(ri *RiskItem) bool {
	switch ri.AppliesWhen {
	case "", "always":
		return true
	case "unincorporated":
		return !r.World.Entity.Incorporated
	case "incorporated":
		return r.World.Entity.Incorporated
	case "has_customers":
		return r.World.Users > 0
	case "has_gstin":
		return r.World.Entity.GSTIN
	case "agency_build":
		return r.World.Build.Agency != ""
	default:
		return false // unknown gate never fires; content authors see it in tests
	}
}

func clamp01(x float64) float64 {
	if x < 0 || math.IsNaN(x) {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
