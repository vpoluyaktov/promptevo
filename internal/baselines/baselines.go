// Package baselines provides deterministic, non-LLM Wordle players used as
// reference curves against the evolving agent. See ARCHITECTURE.md §3.
package baselines

import (
	"math/rand/v2"

	"promptevo/internal/wordle"
)

// Player picks a guess given the game state and the current candidate list.
type Player interface {
	// Guess returns a 5-letter word. candidates is the remaining consistent
	// answer pool; implementations may ignore it (e.g. fixed openers).
	Guess(g *wordle.Game, candidates []string) string
	// Name returns the agent_type label persisted in the games table.
	Name() string
}

// ─── RandomPlayer ─────────────────────────────────────────────────────────────

// RandomPlayer picks a uniformly random remaining candidate (seeded).
type RandomPlayer struct {
	rng *rand.Rand
}

// NewRandomPlayer creates a RandomPlayer with the given seed.
func NewRandomPlayer(seed int64) *RandomPlayer {
	return &RandomPlayer{rng: rand.New(rand.NewPCG(uint64(seed), 1))}
}

func (p *RandomPlayer) Name() string { return "random" }

// Guess picks a random element from candidates; falls back to the first
// word in the game's prior guesses list if candidates is empty (shouldn't happen).
func (p *RandomPlayer) Guess(_ *wordle.Game, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	return candidates[p.rng.IntN(len(candidates))]
}

// ─── FrequencyPlayer ──────────────────────────────────────────────────────────

// FrequencyPlayer picks the candidate with the highest positional letter
// frequency score across the current candidate pool.
type FrequencyPlayer struct{}

// NewFrequencyPlayer creates a FrequencyPlayer.
func NewFrequencyPlayer() *FrequencyPlayer { return &FrequencyPlayer{} }

func (p *FrequencyPlayer) Name() string { return "frequency" }

// Guess picks the candidate that maximizes the sum of positional letter
// frequencies within the current candidate pool.
func (p *FrequencyPlayer) Guess(_ *wordle.Game, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	// Build positional frequency table: freq[pos][letter] = count.
	var freq [wordle.WordLen][26]int
	for _, w := range candidates {
		for i := 0; i < wordle.WordLen; i++ {
			freq[i][w[i]-'a']++
		}
	}

	best := ""
	bestScore := -1
	for _, w := range candidates {
		score := 0
		for i := 0; i < wordle.WordLen; i++ {
			score += freq[i][w[i]-'a']
		}
		if score > bestScore {
			bestScore = score
			best = w
		}
	}
	return best
}

// ─── EntropyPlayer ────────────────────────────────────────────────────────────

// EntropyPlayer picks the guess from the current candidate pool that maximises
// the expected number of information bits (Shannon entropy reduction).
type EntropyPlayer struct{}

// NewEntropyPlayer creates an EntropyPlayer.
func NewEntropyPlayer() *EntropyPlayer { return &EntropyPlayer{} }

func (p *EntropyPlayer) Name() string { return "entropy" }

// Guess iterates over candidates, computing for each one the expected
// information gain and returning the one with the highest expected gain.
// This is O(n²) in the number of candidates; acceptable for small pools.
func (p *EntropyPlayer) Guess(_ *wordle.Game, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	n := len(candidates)
	bestGuess := ""
	bestExpected := -1.0

	for _, guess := range candidates {
		// Partition candidates by the feedback pattern they would produce.
		patternCount := make(map[wordle.Feedback]int, n)
		for _, answer := range candidates {
			fb := wordle.ScoreGuess(guess, answer)
			patternCount[fb]++
		}

		// Expected information gain = sum over patterns of P(pattern)*log2(n/count).
		expected := 0.0
		for _, count := range patternCount {
			p := float64(count) / float64(n)
			expected += p * wordle.InfoGainBits(n, count)
		}

		if expected > bestExpected {
			bestExpected = expected
			bestGuess = guess
		}
	}
	return bestGuess
}
