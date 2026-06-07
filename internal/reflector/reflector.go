// Package reflector rewrites the strategy prompt between generations by asking
// the LLM to self-reflect on generation statistics. See ARCHITECTURE.md §9.5, §11.
package reflector

import (
	"context"
	"fmt"
	"strings"

	"promptevo/internal/llm"
)

// Delimiters used by the reflector for structured output parsing.
const (
	PromptStartDelimiter   = "---PROMPT_START---"
	PromptEndDelimiter     = "---PROMPT_END---"
	SummaryStartDelimiter  = "---SUMMARY_START---"
	SummaryEndDelimiter    = "---SUMMARY_END---"
)

// PriorGeneration is a compact summary of one completed generation for history context.
type PriorGeneration struct {
	GenIndex        int
	SolveRate       float64
	MeanGuesses     float64
	MeanInfoGain    float64
	ViolationRate   float64
	WinDistribution string
	Prompt          string
}

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
	History         []PriorGeneration // all completed generations before this one
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

// Reflect returns the next strategy prompt and a diagnosis summary.
// When the delimited prompt block cannot be parsed, ok is false and the caller
// reuses currentPrompt. summary may be non-empty even when ok is false.
func (r *Reflector) Reflect(ctx context.Context, currentPrompt string, stats GenerationStats) (newPrompt string, summary string, reflection string, ok bool, usage TokenUsage, err error) {
	sysMsg := `You are an AI research assistant improving a Wordle-playing agent's strategy prompt.

Your job: analyse performance data and make SURGICAL changes to the strategy content only.

STRICT RULES:
- Change ONLY game strategy — opening word choices, constraint tracking, information gain, elimination tactics
- Do NOT rewrite grammar, punctuation, sentence structure, or phrasing that is not about strategy
- Do NOT add motivational language, personality, or meta-commentary
- Do NOT restructure sections that already work
- Every change must be directly justified by a specific failure pattern in the data
- KEEP THE PROMPT AS SHORT AS POSSIBLE — every token in the strategy prompt is paid for on every single player move; unnecessary words, repetition, or verbose explanations directly inflate experiment cost with no benefit to game performance; ruthlessly cut any sentence that does not change player behaviour

Output in exactly this order:

1. A diagnosis summary wrapped in these delimiters (3–8 bullet points, plain text, no markdown headers).
   The FIRST bullet MUST report the constraint violation rate and trend (e.g. "• Violation rate: 1.4/game ↑ from 1.2").
   Remaining bullets describe specific failure patterns from the game samples:
---SUMMARY_START---
• Violation rate: X.X/game (↑/↓/= vs prior gen)
• Key failure pattern 1
• …
---SUMMARY_END---

2. The improved strategy prompt wrapped in these delimiters:
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
		return currentPrompt, "", "", false, usage, fmt.Errorf("reflector LLM call: %w", callErr)
	}
	usage.InputTokens = resp.InputTokens
	usage.OutputTokens = resp.OutputTokens

	parsedSummary, _ := ParseSummary(resp.Content)
	parsed, parsedOK := ParsePrompt(resp.Content)
	if !parsedOK {
		return currentPrompt, parsedSummary, resp.Content, false, usage, nil
	}

	return parsed, parsedSummary, resp.Content, true, usage, nil
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

// ParseSummary extracts the text between the SUMMARY delimiters.
// ok is false if the delimiters are absent or the inner text is empty.
func ParseSummary(raw string) (summary string, ok bool) {
	startIdx := strings.Index(raw, SummaryStartDelimiter)
	endIdx := strings.Index(raw, SummaryEndDelimiter)
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return "", false
	}
	inner := strings.TrimSpace(raw[startIdx+len(SummaryStartDelimiter) : endIdx])
	if inner == "" {
		return "", false
	}
	return inner, true
}

// buildReflectorUserMessage formats the prompt for the reflector LLM.
func buildReflectorUserMessage(currentPrompt string, stats GenerationStats) string {
	var sb strings.Builder

	// ── Historical context ────────────────────────────────────────────────────
	if len(stats.History) > 0 {
		sb.WriteString("## History of All Prior Generations\n\n")
		sb.WriteString("Use this to detect trends, avoid repeating failed tactics, and understand what has already been tried.\n\n")
		for _, h := range stats.History {
			sb.WriteString(fmt.Sprintf("### Gen %d — solve %.1f%% | mean guesses %.2f | info gain %.2f bits | violations %.2f/game",
				h.GenIndex, h.SolveRate*100, h.MeanGuesses, h.MeanInfoGain, h.ViolationRate))
			if h.WinDistribution != "" {
				sb.WriteString(fmt.Sprintf(" | dist: %s", h.WinDistribution))
			}
			sb.WriteString("\n")
			sb.WriteString("Prompt used:\n```\n")
			sb.WriteString(h.Prompt)
			sb.WriteString("\n```\n\n")
		}
	}

	sb.WriteString(fmt.Sprintf("## Generation %d Performance (current — needs improvement)\n\n", stats.GenIndex))
	sb.WriteString(fmt.Sprintf("- Solve rate: %.1f%%\n", stats.SolveRate*100))
	sb.WriteString(fmt.Sprintf("- Mean guesses used: %.2f\n", stats.MeanGuesses))
	sb.WriteString(fmt.Sprintf("- Mean information gain: %.2f bits/game\n", stats.MeanInfoGain))
	sb.WriteString(fmt.Sprintf("- Constraint violation rate: %.2f/game\n", stats.ViolationRate))
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
	sb.WriteString("2. Write a diagnosis summary (3–8 bullet points). The FIRST bullet MUST state the constraint violation rate for this generation and whether it improved or worsened vs the prior generation (e.g. '• Violation rate: 1.4/game ↑ from 1.2'). If this is generation 0 with no prior, state it as '• Violation rate: X.X/game (baseline)'. Remaining bullets list key failure patterns.\n")
	sb.WriteString("3. Make targeted changes to the strategy content only — fix the specific tactical weaknesses you identified.\n")
	sb.WriteString("4. Do NOT change grammar, punctuation, or phrasing. Only change strategy instructions.\n")
	sb.WriteString("5. SHORTEN wherever possible — remove any sentence that does not directly change player behaviour; the player pays tokens for every word on every turn.\n")
	sb.WriteString("6. Output in this exact order:\n\n")
	sb.WriteString("---SUMMARY_START---\n")
	sb.WriteString("• Violation rate: X.X/game (↑/↓/= vs prior gen or 'baseline' for gen 0)\n")
	sb.WriteString("• <key failure pattern 1>\n")
	sb.WriteString("• <key failure pattern 2>\n")
	sb.WriteString("---SUMMARY_END---\n\n")
	sb.WriteString("---PROMPT_START---\n")
	sb.WriteString("<updated strategy prompt>\n")
	sb.WriteString("---PROMPT_END---\n")

	return sb.String()
}
