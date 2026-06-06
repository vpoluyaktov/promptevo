// Package wordle_test contains golden tests for the Wordle scoring engine.
//
// All expected Feedback values were derived by hand-tracing the two-pass
// algorithm (ARCHITECTURE.md §9.1):
//
//	Pass 1 — assign Green for every positional match; decrement per-letter budget.
//	Pass 2 — for each remaining position left-to-right: Yellow if budget > 0,
//	          otherwise Gray.
//
// Note: some values in the ARCHITECTURE.md §9.1 table are incorrect (e.g.
// slate/crane is listed as XXYXG but the algorithm gives XXGXG because 'a' at
// index 2 is an exact positional match).  The principle — not the table — is
// the spec.  All cases below were derived from first principles.
package wordle_test

import (
	"math"
	"testing"

	"promptevo/internal/wordle"
)

// makeFB builds a Feedback from a 5-char GYX string for test readability.
func makeFB(s string) wordle.Feedback {
	if len(s) != wordle.WordLen {
		panic("makeFB: string must be exactly 5 chars")
	}
	var f wordle.Feedback
	for i, c := range s {
		switch c {
		case 'G':
			f[i] = wordle.Green
		case 'Y':
			f[i] = wordle.Yellow
		default:
			f[i] = wordle.Gray
		}
	}
	return f
}

// --- ScoreGuess golden tests ---

func TestScoreGuess(t *testing.T) {
	tests := []struct {
		name   string
		guess  string
		answer string
		want   string // GYX 5-char encoding
	}{
		// ── trivial cases ──────────────────────────────────────────────────
		{
			name:   "all_correct",
			guess:  "crane",
			answer: "crane",
			want:   "GGGGG",
		},
		{
			name:   "all_correct_with_repeats",
			guess:  "abcde",
			answer: "abcde",
			want:   "GGGGG",
		},
		{
			// thorn / smile: no shared letters at all
			name:   "all_gray_no_shared_letters",
			guess:  "thorn",
			answer: "smile",
			want:   "XXXXX",
		},
		{
			// adore / oared: no positional match, all 5 letters present in answer
			// Pass 1: none
			// Pass 2: a[0]→Y, d[1]→Y, o[2]→Y, r[3]→Y, e[4]→Y
			name:   "all_yellow_no_positional_match",
			guess:  "adore",
			answer: "oared",
			want:   "YYYYY",
		},

		// ── architecture golden table §9.1 – algorithm-verified ───────────
		{
			// babes: b(0)a(1)b(2)e(3)s(4)  abbey: a(0)b(1)b(2)e(3)y(4)
			// Pass 1: b[2]→G (rem[b]=1), e[3]→G (rem[e]=0)
			// Pass 2: b[0]→Y (rem[b]=1→0), a[1]→Y (rem[a]=1→0), s[4]→X
			name:   "arch_babes_abbey",
			guess:  "babes",
			answer: "abbey",
			want:   "YYGGX",
		},
		{
			// speed: s(0)p(1)e(2)e(3)d(4)  abide: a(0)b(1)i(2)d(3)e(4)
			// Pass 1: no exact matches
			// Pass 2: s→X, p→X, e[2]→Y (rem[e]=1→0), e[3]→X (budget 0), d[4]→Y
			name:   "arch_speed_abide",
			guess:  "speed",
			answer: "abide",
			want:   "XXYXY",
		},
		{
			// aaaaa / crane: crane has one 'a' at position 2 (exact match → Green).
			// All other 'a' positions have no remaining budget → Gray.
			// NOTE: ARCHITECTURE.md table shows XGXXX which is wrong — crane[1]='r'.
			name:   "arch_aaaaa_crane",
			guess:  "aaaaa",
			answer: "crane",
			want:   "XXGXX",
		},
		{
			// eevee / crane: crane has one 'e' at pos 4. eevee[4]='e' → Green.
			// rem[e] drops to 0; eevee[0..3] are all Gray.
			// NOTE: ARCHITECTURE.md table shows YXXXX (wrong); XXXXG is correct.
			name:   "arch_eevee_crane",
			guess:  "eevee",
			answer: "crane",
			want:   "XXXXG",
		},
		{
			// geese / eject: eject has e:2. eject[2]='e' is exact with geese[2]→Green
			// (rem[e]=1). geese[1]='e' → Yellow (rem[e]=1→0). geese[4]='e' → X.
			// NOTE: ARCHITECTURE.md table shows XYXXY* (wrong); XYGXX is correct.
			name:   "arch_geese_eject",
			guess:  "geese",
			answer: "eject",
			want:   "XYGXX",
		},

		// ── two-pass duplicate-letter algorithm ───────────────────────────
		{
			// crane / arise: r[1] and e[4] are exact → Green. a[2] of crane hits
			// 'a' in arise (budget 1) → Yellow. c and n absent → Gray.
			name:   "crane_arise",
			guess:  "crane",
			answer: "arise",
			want:   "XGYXG",
		},
		{
			// speed / spell: s[0], p[1], e[2] exact → GGG. spell has one 'e'; budget
			// consumed by green. Second e[3] → X. d absent → X.
			name:   "speed_spell",
			guess:  "speed",
			answer: "spell",
			want:   "GGGXX",
		},
		{
			// sleep / spell: s[0] and e[2] exact → G_G__. spell has l:2; l[1]→Y
			// (rem[l]=2→1). e[3]→X (rem[e]=0). p[4]→Y (rem[p]=1→0).
			name:   "sleep_spell",
			guess:  "sleep",
			answer: "spell",
			want:   "GYGXY",
		},
		{
			// eerie / erase: erase has e:2 (pos 0,4). Both match eerie[0] and
			// eerie[4] → Green (rem[e]=0). eerie[1]='e'→X. eerie[2]='r'→Y
			// (rem[r]=1→0). eerie[3]='i' absent → X.
			name:   "eerie_erase",
			guess:  "eerie",
			answer: "erase",
			want:   "GXYXG",
		},
		{
			// abbey / kebab: b[2] exact → Green (rem[b]=1). a[0]→Y (rem[a]=1→0),
			// b[1]→Y (rem[b]=1→0), e[3]→Y (rem[e]=1→0), y[4] absent → X.
			name:   "abbey_kebab",
			guess:  "abbey",
			answer: "kebab",
			want:   "YYGYX",
		},
		{
			// speed / creep: e[2] and e[3] exact → GG (rem[e]=0). p[1]→Y
			// (rem[p]=1→0). s[0] and d[4] absent → X.
			name:   "speed_creep",
			guess:  "speed",
			answer: "creep",
			want:   "XYGGX",
		},
		{
			// speed / greed: e[2], e[3], d[4] exact → GGG. s and p absent → X.
			name:   "speed_greed",
			guess:  "speed",
			answer: "greed",
			want:   "XXGGG",
		},
		{
			// spree / creed: e[3] exact → G (rem[e]=1). r[2]→Y (rem[r]=1→0),
			// e[4]→Y (rem[e]=1→0). s and p absent → X.
			name:   "spree_creed",
			guess:  "spree",
			answer: "creed",
			want:   "XXYGY",
		},

		// ── more duplicate-letter boundary cases ──────────────────────────
		{
			// aabcd / aaxyz: a[0] and a[1] both exact → GG (rem[a]=0).
			// b, c, d absent from aaxyz → X.
			name:   "duplicate_both_green",
			guess:  "aabcd",
			answer: "aaxyz",
			want:   "GGXXX",
		},
		{
			// llama / alarm: l[1] exact → G (rem[l]=0). a[2] exact → G (rem[a]=1).
			// l[0]→X (rem[l]=0). m[3]→Y (rem[m]=1→0). a[4]→Y (rem[a]=1→0).
			name:   "llama_alarm",
			guess:  "llama",
			answer: "alarm",
			want:   "XGGYY",
		},
		{
			// aabcd / baabc: a[1] exact → G (rem[a]=1). a[0]→Y (rem[a]=1→0),
			// b[2]→Y (rem[b]=2→1), c[3]→Y (rem[c]=1→0), d[4] absent → X.
			name:   "aabcd_baabc",
			guess:  "aabcd",
			answer: "baabc",
			want:   "YGYYX",
		},
		{
			// swear / resaw: a[3] exact → G. s→Y, w→Y, e→Y, r→Y.
			name:   "swear_resaw",
			guess:  "swear",
			answer: "resaw",
			want:   "YYYGY",
		},

		// ── boundary / invalid input (spec §9.1) ──────────────────────────
		{
			name:   "empty_guess",
			guess:  "",
			answer: "crane",
			want:   "XXXXX",
		},
		{
			name:   "empty_answer",
			guess:  "crane",
			answer: "",
			want:   "XXXXX",
		},
		{
			name:   "wrong_length_short_guess",
			guess:  "cran",
			answer: "crane",
			want:   "XXXXX",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wordle.ScoreGuess(tc.guess, tc.answer)
			if got.String() != tc.want {
				t.Errorf("ScoreGuess(%q, %q) = %q, want %q",
					tc.guess, tc.answer, got.String(), tc.want)
			}
		})
	}
}

// --- Feedback.String() ---

func TestFeedbackString(t *testing.T) {
	tests := []struct {
		name string
		fb   wordle.Feedback
		want string
	}{
		{
			name: "all_green",
			fb:   wordle.Feedback{wordle.Green, wordle.Green, wordle.Green, wordle.Green, wordle.Green},
			want: "GGGGG",
		},
		{
			name: "all_yellow",
			fb:   wordle.Feedback{wordle.Yellow, wordle.Yellow, wordle.Yellow, wordle.Yellow, wordle.Yellow},
			want: "YYYYY",
		},
		{
			name: "all_gray",
			fb:   wordle.Feedback{wordle.Gray, wordle.Gray, wordle.Gray, wordle.Gray, wordle.Gray},
			want: "XXXXX",
		},
		{
			// zero Feedback value maps to all-Gray
			name: "zero_value_is_all_gray",
			fb:   wordle.Feedback{},
			want: "XXXXX",
		},
		{
			name: "mixed_GXYXG",
			fb:   wordle.Feedback{wordle.Green, wordle.Gray, wordle.Yellow, wordle.Gray, wordle.Green},
			want: "GXYXG",
		},
		{
			name: "alternating_GYGYG",
			fb:   wordle.Feedback{wordle.Green, wordle.Yellow, wordle.Green, wordle.Yellow, wordle.Green},
			want: "GYGYG",
		},
		{
			name: "single_green_first",
			fb:   wordle.Feedback{wordle.Green, wordle.Gray, wordle.Gray, wordle.Gray, wordle.Gray},
			want: "GXXXX",
		},
		{
			name: "single_yellow_last",
			fb:   wordle.Feedback{wordle.Gray, wordle.Gray, wordle.Gray, wordle.Gray, wordle.Yellow},
			want: "XXXXY",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fb.String()
			if got != tc.want {
				t.Errorf("Feedback.String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- InfoGainBits ---

// approxEq checks floating-point equality within a tight tolerance.
func approxEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestInfoGainBits(t *testing.T) {
	tests := []struct {
		name   string
		before int
		after  int
		want   float64
	}{
		{
			// before == 0 → return 0 (spec §9.3: no information definable)
			name:   "before_zero",
			before: 0,
			after:  5,
			want:   0,
		},
		{
			// after == before → 0 bits (guess eliminated nothing)
			name:   "no_elimination",
			before: 10,
			after:  10,
			want:   0,
		},
		{
			// after == 0 → treat as 1 to avoid +Inf (spec §9.3)
			name:   "after_zero_treated_as_one",
			before: 10,
			after:  0,
			want:   math.Log2(10),
		},
		{
			// after == 1 → log2(before/1) = log2(before)
			name:   "after_one",
			before: 10,
			after:  1,
			want:   math.Log2(10),
		},
		{
			// Halved → exactly 1 bit
			name:   "half_remaining",
			before: 10,
			after:  5,
			want:   1.0,
		},
		{
			// Quarter remaining → 2 bits
			name:   "quarter_remaining",
			before: 16,
			after:  4,
			want:   2.0,
		},
		{
			// Single candidate, unchanged → 0 bits
			name:   "single_unchanged",
			before: 1,
			after:  1,
			want:   0,
		},
		{
			// Guess equals answer: after = 1, gain = log2(before)
			name:   "guess_equals_answer_128",
			before: 128,
			after:  1,
			want:   7.0,
		},
		{
			// Two candidates → one left = 1 bit
			name:   "two_to_one",
			before: 2,
			after:  1,
			want:   1.0,
		},
		{
			// 8 → 2 = log2(4) = 2 bits
			name:   "eight_to_two",
			before: 8,
			after:  2,
			want:   2.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wordle.InfoGainBits(tc.before, tc.after)
			if !approxEq(got, tc.want) {
				t.Errorf("InfoGainBits(%d, %d) = %v, want %v",
					tc.before, tc.after, got, tc.want)
			}
		})
	}
}

// --- FilterCandidates ---

func TestFilterCandidates(t *testing.T) {
	tests := []struct {
		name       string
		candidates []string
		guess      string
		feedback   wordle.Feedback
		want       []string // unordered
	}{
		{
			name:       "empty_slice",
			candidates: []string{},
			guess:      "crane",
			feedback:   makeFB("GGGGG"),
			want:       []string{},
		},
		{
			name:       "nil_slice",
			candidates: nil,
			guess:      "crane",
			feedback:   makeFB("GGGGG"),
			want:       []string{},
		},
		{
			// The exact answer is always self-consistent.
			name:       "single_candidate_is_answer",
			candidates: []string{"crane"},
			guess:      "crane",
			feedback:   makeFB("GGGGG"),
			want:       []string{"crane"},
		},
		{
			// "crank" and "craze" don't score GGGGG against guess "crane" (they
			// differ at the last position), so only "crane" survives.
			name:       "all_green_keeps_only_exact_answer",
			candidates: []string{"crane", "crank", "craze"},
			guess:      "crane",
			feedback:   makeFB("GGGGG"),
			want:       []string{"crane"},
		},
		{
			// speed/spell=GGGXX; both "spell" and "specs" (s,p,e,c,s) give GGGXX
			// when scored against guess "speed". "abcde" does not.
			//
			// Verification for "specs":
			//   speed vs specs: s[0]→G,p[1]→G,e[2]→G(rem[e]=0); e[3]→X,d[4]→X → GGGXX
			// Verification for "abcde":
			//   speed vs abcde: no greens at all; e→Y, d→Y → XXYXY ≠ GGGXX
			name:       "multiple_consistent_candidates",
			candidates: []string{"spell", "specs", "abcde"},
			guess:      "speed",
			feedback:   makeFB("GGGXX"),
			want:       []string{"spell", "specs"},
		},
		{
			// No candidate is consistent with GGGGG for a guess that doesn't
			// match any of the listed candidates exactly.
			name:       "none_consistent",
			candidates: []string{"abide", "crank"},
			guess:      "speed",
			feedback:   makeFB("GGGGG"),
			want:       []string{},
		},
		{
			// The true answer always survives its own feedback.
			name:       "answer_in_pool_always_survives",
			candidates: []string{"abbey"},
			guess:      "babes",
			feedback:   makeFB("YYGGX"),
			want:       []string{"abbey"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := wordle.FilterCandidates(tc.candidates, tc.guess, tc.feedback)
			if got == nil {
				got = []string{}
			}

			if len(got) != len(tc.want) {
				t.Errorf("FilterCandidates: len=%d %v, want len=%d %v",
					len(got), got, len(tc.want), tc.want)
				return
			}

			wantSet := make(map[string]bool, len(tc.want))
			for _, w := range tc.want {
				wantSet[w] = true
			}
			for _, g := range got {
				if !wantSet[g] {
					t.Errorf("FilterCandidates: unexpected candidate %q in result %v", g, got)
				}
			}
		})
	}
}

// --- Game struct behaviour ---
//
// AddGuess / IsWon / IsOver are not yet in the stub (ARCHITECTURE.md marks them
// TODO for the Backend Developer). The tests below use direct struct manipulation
// to document the expected state invariants. Once the Backend adds those methods,
// add table-driven tests here calling them directly.

func TestGameInitialState(t *testing.T) {
	g := &wordle.Game{
		Answer:   "crane",
		MaxTurns: wordle.MaxTurns,
	}
	if g.Won {
		t.Error("new game should not be Won")
	}
	if len(g.Guesses) != 0 {
		t.Errorf("new game: Guesses len = %d, want 0", len(g.Guesses))
	}
	if len(g.Feedbacks) != 0 {
		t.Errorf("new game: Feedbacks len = %d, want 0", len(g.Feedbacks))
	}
	if g.MaxTurns != wordle.MaxTurns {
		t.Errorf("MaxTurns = %d, want %d", g.MaxTurns, wordle.MaxTurns)
	}
}

func TestGameSixWrongGuessesNotWon(t *testing.T) {
	g := &wordle.Game{Answer: "crane", MaxTurns: wordle.MaxTurns}
	wrong := "thorn" // no letters shared with "crane"
	for i := 0; i < wordle.MaxTurns; i++ {
		g.Guesses = append(g.Guesses, wrong)
		g.Feedbacks = append(g.Feedbacks, wordle.ScoreGuess(wrong, g.Answer))
	}
	if g.Won {
		t.Error("six wrong guesses should not set Won=true")
	}
	if len(g.Guesses) != wordle.MaxTurns {
		t.Errorf("expected %d guesses recorded, got %d", wordle.MaxTurns, len(g.Guesses))
	}
}

func TestGameCorrectGuessFeedbackAllGreen(t *testing.T) {
	g := &wordle.Game{Answer: "crane", MaxTurns: wordle.MaxTurns}
	fb := wordle.ScoreGuess("crane", g.Answer)
	g.Guesses = append(g.Guesses, "crane")
	g.Feedbacks = append(g.Feedbacks, fb)

	if fb.String() != "GGGGG" {
		t.Errorf("correct guess should produce GGGGG, got %q", fb.String())
	}
	// Correct guess → all Green tiles
	for i, tr := range fb {
		if tr != wordle.Green {
			t.Errorf("tile %d: got %v, want Green", i, tr)
		}
	}
}

func TestGameFeedbacksMatchGuessCount(t *testing.T) {
	g := &wordle.Game{Answer: "abbey", MaxTurns: wordle.MaxTurns}
	guesses := []string{"crane", "babes", "abbey"}
	for _, gu := range guesses {
		g.Guesses = append(g.Guesses, gu)
		g.Feedbacks = append(g.Feedbacks, wordle.ScoreGuess(gu, g.Answer))
	}
	if len(g.Guesses) != len(g.Feedbacks) {
		t.Errorf("Guesses len %d != Feedbacks len %d", len(g.Guesses), len(g.Feedbacks))
	}
	// Last feedback should be GGGGG (answer == guess)
	last := g.Feedbacks[len(g.Feedbacks)-1]
	if last.String() != "GGGGG" {
		t.Errorf("last feedback = %q, want GGGGG", last.String())
	}
}

func TestMaxTurnsConstant(t *testing.T) {
	if wordle.MaxTurns != 6 {
		t.Errorf("MaxTurns = %d, want 6", wordle.MaxTurns)
	}
}

func TestWordLenConstant(t *testing.T) {
	if wordle.WordLen != 5 {
		t.Errorf("WordLen = %d, want 5", wordle.WordLen)
	}
}
