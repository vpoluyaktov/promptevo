// Package agent turns game state plus the current strategy prompt into a single
// Wordle guess by calling the LLM. See ARCHITECTURE.md §3 and §9.4.
package agent

import (
	"context"

	"promptevo/internal/llm"
	"promptevo/internal/wordle"
)

// TokenUsage records input/output tokens for one LLM call.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// Agent is the LLM-backed Wordle player.
type Agent struct {
	Client      llm.Client
	Lists       *wordle.WordLists
	Model       string
	Temperature float64
}

// NextGuess produces the next guess for game g given the current strategyPrompt.
// On an invalid/contradictory model output, violation is true and a deterministic
// valid candidate is substituted so the game proceeds (ARCHITECTURE.md §9.4).
// TODO(backend): implement prompt construction, LLM call, and parsing.
func (a *Agent) NextGuess(ctx context.Context, strategyPrompt string, g *wordle.Game) (guess string, reasoning string, violation bool, usage TokenUsage, err error) {
	return "", "", false, TokenUsage{}, nil
}
