// Package reflector rewrites the strategy prompt between generations by asking
// the LLM to self-reflect on generation statistics. See ARCHITECTURE.md §9.5, §11.
package reflector

import (
	"context"

	"promptevo/internal/llm"
)

// PromptStartDelimiter and PromptEndDelimiter wrap the reflector's rewritten
// prompt. The parser extracts the text strictly between them.
const (
	PromptStartDelimiter = "---PROMPT_START---"
	PromptEndDelimiter   = "---PROMPT_END---"
)

// GenerationStats is the aggregate fed to the reflector.
type GenerationStats struct {
	GenIndex      int
	SolveRate     float64
	MeanGuesses   float64
	MeanInfoGain  float64
	ViolationRate float64
	// FailedSamples are a few representative lost games (answer + guess/feedback).
	FailedSamples []string
}

// TokenUsage records input/output tokens for one LLM call.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// Reflector produces an improved strategy prompt from the prior one + stats.
type Reflector struct {
	Client      llm.Client
	Model       string
	Temperature float64
}

// Reflect returns the next strategy prompt. When the delimited block cannot be
// parsed, ok is false and the caller reuses currentPrompt (ARCHITECTURE.md §9.5).
// TODO(backend): implement prompt construction, LLM call, and delimiter parsing.
func (r *Reflector) Reflect(ctx context.Context, currentPrompt string, stats GenerationStats) (newPrompt string, reflection string, ok bool, usage TokenUsage, err error) {
	return currentPrompt, "", false, TokenUsage{}, nil
}

// ParsePrompt extracts the text between the PROMPT delimiters. ok is false if
// either delimiter is missing, reversed, or the inner text is empty.
// TODO(backend): implement.
func ParsePrompt(raw string) (prompt string, ok bool) {
	return "", false
}
