// Package baselines provides deterministic, non-LLM Wordle players used as
// reference curves against the evolving agent. See ARCHITECTURE.md §3.
package baselines

import "promptevo/internal/wordle"

// Player picks a guess given the game state and the current candidate list.
type Player interface {
	// Guess returns a 5-letter word. candidates is the remaining consistent
	// answer pool; implementations may ignore it (e.g. fixed openers).
	Guess(g *wordle.Game, candidates []string) string
	// Name returns the agent_type label persisted in the games table.
	Name() string
}

// RandomPlayer picks a uniformly random remaining candidate (seeded).
// TODO(backend): implement with a seeded *rand.Rand.
type RandomPlayer struct{}

func (RandomPlayer) Name() string { return "random" }

func (RandomPlayer) Guess(g *wordle.Game, candidates []string) string { return "" }

// FrequencyPlayer picks the candidate maximizing positional letter frequency.
// TODO(backend): implement.
type FrequencyPlayer struct{}

func (FrequencyPlayer) Name() string { return "frequency" }

func (FrequencyPlayer) Guess(g *wordle.Game, candidates []string) string { return "" }

// EntropyPlayer picks the guess maximizing expected information gain.
// TODO(backend): implement.
type EntropyPlayer struct{}

func (EntropyPlayer) Name() string { return "entropy" }

func (EntropyPlayer) Guess(g *wordle.Game, candidates []string) string { return "" }
