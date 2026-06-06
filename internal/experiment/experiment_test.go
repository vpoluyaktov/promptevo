// Package experiment_test verifies the Convergence classifier and other
// pure functions in the experiment package.
package experiment_test

import (
	"testing"

	"promptevo/internal/experiment"
)

// TestConvergence covers all four branches defined in ARCHITECTURE.md §9.6:
//
//	< 3 generations completed   → "improving"
//	max-min < 0.02              → "stable"
//	direction reverses          → "oscillating"
//	monotonic trend ≥ 0.02 band → "improving"
func TestConvergence(t *testing.T) {
	tests := []struct {
		name       string
		solveRates []float64
		want       string
	}{
		// ── fewer than 3 generations ───────────────────────────────────────
		{
			name:       "zero_gens",
			solveRates: []float64{},
			want:       "improving",
		},
		{
			name:       "one_gen",
			solveRates: []float64{0.60},
			want:       "improving",
		},
		{
			name:       "two_gens",
			solveRates: []float64{0.50, 0.65},
			want:       "improving",
		},

		// ── stable (max-min < 0.02, last 3 only) ──────────────────────────
		{
			name:       "stable_flat",
			solveRates: []float64{0.60, 0.60, 0.60},
			want:       "stable",
		},
		{
			name:       "stable_tiny_variation",
			solveRates: []float64{0.50, 0.51, 0.50},
			want:       "stable",
		},
		{
			// max=0.510, min=0.500 → delta=0.010 < 0.02 → stable
			name:       "stable_within_band",
			solveRates: []float64{0.500, 0.510, 0.505},
			want:       "stable",
		},
		{
			// Extra generations: only the last 3 matter.
			// Last 3: [0.80, 0.81, 0.80] → delta=0.01 → stable
			name:       "stable_considers_only_last_3",
			solveRates: []float64{0.10, 0.50, 0.80, 0.81, 0.80},
			want:       "stable",
		},

		// ── oscillating ───────────────────────────────────────────────────
		{
			// g2-g1 > 0, g3-g2 < 0 → direction reverses → oscillating
			name:       "oscillating_up_then_down",
			solveRates: []float64{0.50, 0.70, 0.55},
			want:       "oscillating",
		},
		{
			// g2-g1 < 0, g3-g2 > 0 → oscillating
			name:       "oscillating_down_then_up",
			solveRates: []float64{0.70, 0.50, 0.65},
			want:       "oscillating",
		},
		{
			// Same oscillation but beyond the 3-gen window.
			name:       "oscillating_last_3_of_5",
			solveRates: []float64{0.40, 0.40, 0.60, 0.80, 0.60},
			want:       "oscillating",
		},

		// ── improving (monotonic, outside stability band) ──────────────────
		{
			// Strict improvement, delta > 0.02
			name:       "improving_monotonic_up",
			solveRates: []float64{0.50, 0.60, 0.70},
			want:       "improving",
		},
		{
			// Strict decline is also "improving" per spec (monotonic trend).
			name:       "improving_monotonic_down",
			solveRates: []float64{0.70, 0.60, 0.50},
			want:       "improving",
		},
		{
			// Large jump, definitely outside the stability band.
			name:       "improving_large_jump",
			solveRates: []float64{0.20, 0.50, 0.90},
			want:       "improving",
		},
		{
			// Last 3 of a longer run are monotonically improving.
			name:       "improving_last_3_of_6",
			solveRates: []float64{0.30, 0.20, 0.25, 0.40, 0.55, 0.70},
			want:       "improving",
		},

		// ── boundary: exactly at the 0.02 stability threshold ─────────────
		{
			// delta = 0.02 exactly: spec says < 0.02 is stable, so 0.02 is NOT stable.
			// Direction reverses (0.60 → 0.62 → 0.60): oscillating.
			name:       "exactly_at_threshold_oscillating",
			solveRates: []float64{0.60, 0.62, 0.60},
			want:       "oscillating",
		},
		{
			// delta = 0.019 < 0.02 → stable
			name:       "just_under_threshold_stable",
			solveRates: []float64{0.600, 0.619, 0.600},
			want:       "stable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := experiment.Convergence(tc.solveRates)
			if got != tc.want {
				t.Errorf("Convergence(%v) = %q, want %q", tc.solveRates, got, tc.want)
			}
		})
	}
}
