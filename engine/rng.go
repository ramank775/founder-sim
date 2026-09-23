package engine

import "math"

// Randomness is owned by the engine. Every draw is a pure function of
// (run seed, draw index), so a run can be saved, reloaded and replayed
// without storing generator state, and two players with the same seed and
// the same actions get the same rolls. The LLM never rolls anything.

func mix64(x uint64) uint64 {
	// splitmix64 finaliser
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// nextU64 returns the next raw 64-bit draw and advances the sequence.
func (r *Run) nextU64() uint64 {
	v := mix64(uint64(r.Seed) ^ mix64(r.RollSeq))
	r.RollSeq++
	return v
}

// Float returns a draw in [0,1).
func (r *Run) Float() float64 {
	return float64(r.nextU64()>>11) / float64(1<<53)
}

// Roll returns true with probability p (clamped to [0,1]).
func (r *Run) Roll(p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 1 {
		return true
	}
	return r.Float() < p
}

// IntN returns a draw in [0,n). n <= 0 returns 0.
func (r *Run) IntN(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.nextU64() % uint64(n))
}

// Between returns a draw in [lo,hi] inclusive.
func (r *Run) Between(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + r.IntN(hi-lo+1)
}

// Pick chooses an index from weights proportionally. Non-positive or NaN
// weights are treated as zero. If all are zero, returns 0.
func (r *Run) Pick(weights []float64) int {
	total := 0.0
	for _, w := range weights {
		if w > 0 && !math.IsNaN(w) && !math.IsInf(w, 0) {
			total += w
		}
	}
	if total == 0 {
		return 0
	}
	x := r.Float() * total
	acc := 0.0
	for i, w := range weights {
		if w > 0 && !math.IsNaN(w) && !math.IsInf(w, 0) {
			acc += w
			if x < acc {
				return i
			}
		}
	}
	return len(weights) - 1
}
