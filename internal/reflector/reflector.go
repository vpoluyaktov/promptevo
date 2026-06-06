// Package reflector rewrites the strategy prompt between generations by asking
// the LLM to self-reflect on generation statistics. See ARCHITECTURE.md §9.5, §11.
package reflector

import (
	"context"
	"fmt"
	"strings"

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
func (r *Reflector) Reflect(ctx context.Context, currentPrompt string, stats GenerationStats) (newPrompt string, reflection string, ok bool, usage TokenUsage, err error) {
	sysMsg := `You are an AI research assistant helping improve a Wordle-playing agent's strategy.
Your job is to analyse the agent's performance and rewrite its strategy prompt to improve future results.

You MUST output the improved strategy prompt wrapped in these exact delimiters (with no other text between them):
---PROMPT_START---
<your improved strategy prompt here>
---PROMPT_END---

The improved prompt should be between 100 and 4000 characters.`

	userMsg := buildReflectorUserMessage(currentPrompt, stats)

	req := llm.CompletionRequest{
		Model: r.Model,
		Messages: []llm.Message{
			{Role: "system", Content: sysMsg},
			{Role: "user", Content: userMsg},
		},
		Temperature: r.Temperature,
		MaxTokens:   2000,
	}

	resp, callErr := r.Client.Complete(ctx, req)
	if callErr != nil {
		return currentPrompt, "", false, usage, fmt.Errorf("reflector LLM call: %w", callErr)
	}
	usage.InputTokens = resp.InputTokens
	usage.OutputTokens = resp.OutputTokens

	parsed, parsedOK := ParsePrompt(resp.Content)
	if !parsedOK {
		return currentPrompt, resp.Content, false, usage, nil
	}

	return parsed, resp.Content, true, usage, nil
}

// ParsePrompt extracts the text between the PROMPT delimiters. ok is false if
// either delimiter is missing, reversed, or the inner text is empty, shorter
// than 50 chars, or longer than 4000 chars.
func ParsePrompt(raw string) (prompt string, ok bool) {
	startIdx := strings.Index(raw, PromptStartDelimiter)
	endIdx := strings.Index(raw, PromptEndDelimiter)

	if startIdx == -1 || endIdx == -1 {
		return "", false
	}
	if endIdx <= startIdx {
		return "", false
	}

	// Extract between delimiters.
	inner := raw[startIdx+len(PromptStartDelimiter) : endIdx]
	inner = strings.TrimSpace(inner)

	if len(inner) < 50 || len([]rune(inner)) > 4000 {
		return "", false
	}

	return inner, true
}

// buildReflectorUserMessage formats the prompt for the reflector LLM.
func buildReflectorUserMessage(currentPrompt string, stats GenerationStats) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Generation %d Performance Report\n\n", stats.GenIndex))
	sb.WriteString(fmt.Sprintf("- Solve rate: %.1f%%\n", stats.SolveRate*100))
	sb.WriteString(fmt.Sprintf("- Mean guesses: %.2f (lower is better; target ≤4.0)\n", stats.MeanGuesses))
	sb.WriteString(fmt.Sprintf("- Mean information gain: %.2f bits per game\n", stats.MeanInfoGain))
	sb.WriteString(fmt.Sprintf("- Violation rate: %.1f%% (invalid/contradictory guesses)\n\n", stats.ViolationRate*100))

	if len(stats.FailedSamples) > 0 {
		sb.WriteString("## Representative Failed Games\n\n")
		for _, sample := range stats.FailedSamples {
			sb.WriteString(sample)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## Current Strategy Prompt\n\n")
	sb.WriteString("```\n")
	sb.WriteString(currentPrompt)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Your Task\n\n")
	sb.WriteString("1. Diagnose the main failure modes in the performance data above.\n")
	sb.WriteString("2. Rewrite the strategy prompt to address these weaknesses.\n")
	sb.WriteString("3. Output your improved prompt between the required delimiters:\n\n")
	sb.WriteString("---PROMPT_START---\n")
	sb.WriteString("<improved strategy prompt>\n")
	sb.WriteString("---PROMPT_END---\n")

	return sb.String()
}
