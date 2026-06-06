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
	GenIndex        int
	SolveRate       float64
	MeanGuesses     float64
	MeanInfoGain    float64
	ViolationRate   float64
	WinDistribution string   // e.g. "turn 1: 2 | turn 2: 4 | lost: 4"
	FailedSamples   []string // up to 3 lost games with per-turn reasoning
	WonSamples      []string // up to 2 won games showing successful constraint tracking
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
	sysMsg := `You are an AI research assistant improving a Wordle-playing agent's strategy prompt.

Your job: analyse performance data and make SURGICAL changes to the strategy content only.

STRICT RULES:
- Change ONLY game strategy — opening word choices, constraint tracking, information gain, elimination tactics
- Do NOT rewrite grammar, punctuation, sentence structure, or phrasing that is not about strategy
- Do NOT add motivational language, personality, or meta-commentary
- Do NOT restructure sections that already work
- Every change must be directly justified by a specific failure pattern in the data

Output the improved prompt wrapped in these exact delimiters:
---PROMPT_START---
<improved strategy prompt>
---PROMPT_END---

The prompt must be 100–4000 characters.`

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

	sb.WriteString(fmt.Sprintf("## Generation %d Performance\n\n", stats.GenIndex))
	sb.WriteString(fmt.Sprintf("- Solve rate: %.1f%%\n", stats.SolveRate*100))
	sb.WriteString(fmt.Sprintf("- Mean guesses used: %.2f\n", stats.MeanGuesses))
	sb.WriteString(fmt.Sprintf("- Mean information gain: %.2f bits/game\n", stats.MeanInfoGain))
	sb.WriteString(fmt.Sprintf("- Constraint violation rate: %.1f%%\n", stats.ViolationRate*100))
	if stats.WinDistribution != "" {
		sb.WriteString(fmt.Sprintf("- Win distribution: %s\n", stats.WinDistribution))
	}
	sb.WriteString("\n")

	if len(stats.WonSamples) > 0 {
		sb.WriteString("## Examples of Won Games (successful constraint tracking)\n\n")
		for _, sample := range stats.WonSamples {
			sb.WriteString(sample)
			sb.WriteString("\n\n")
		}
	}

	if len(stats.FailedSamples) > 0 {
		sb.WriteString("## Examples of Lost Games (diagnose what went wrong)\n\n")
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
	sb.WriteString("1. Identify specific strategic failures in the lost game examples above.\n")
	sb.WriteString("2. Make targeted changes to the strategy content only — fix the specific tactical weaknesses you identified.\n")
	sb.WriteString("3. Do NOT change grammar, punctuation, or phrasing. Only change strategy instructions.\n")
	sb.WriteString("4. Output the updated prompt between the required delimiters:\n\n")
	sb.WriteString("---PROMPT_START---\n")
	sb.WriteString("<updated strategy prompt>\n")
	sb.WriteString("---PROMPT_END---\n")

	return sb.String()
}
