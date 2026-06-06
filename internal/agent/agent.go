// Package agent turns game state plus the current strategy prompt into a single
// Wordle guess by calling the LLM. See ARCHITECTURE.md §3 and §9.4.
package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

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

// guessLineRe matches lines of the form "GUESS: <word>" (case-insensitive).
var guessLineRe = regexp.MustCompile(`(?i)GUESS:\s*([A-Za-z]{5})`)

// standalone5Re matches any standalone 5-letter alphabetic token.
var standalone5Re = regexp.MustCompile(`\b([A-Za-z]{5})\b`)

// NextGuess produces the next guess for game g given the current strategyPrompt.
// It builds a prompt from the strategy + board state, calls the LLM, parses the
// result, and retries up to 3 times when the word is not in the valid-guess list.
// On an invalid/contradictory output, violation is true and the first remaining
// candidate is substituted so the game proceeds (ARCHITECTURE.md §9.4).
func (a *Agent) NextGuess(ctx context.Context, strategyPrompt string, g *wordle.Game) (guess string, reasoning string, violation bool, usage TokenUsage, err error) {
	// Compute current candidate pool.
	candidates := make([]string, len(a.Lists.Answers))
	copy(candidates, a.Lists.Answers)
	for i, prevGuess := range g.Guesses {
		candidates = wordle.FilterCandidates(candidates, prevGuess, g.Feedbacks[i])
	}

	userMsg := buildUserMessage(g, candidates)

	var lastReply string
	for attempt := 0; attempt < 3; attempt++ {
		req := llm.CompletionRequest{
			Model: a.Model,
			Messages: []llm.Message{
				{Role: "system", Content: strategyPrompt},
				{Role: "user", Content: userMsg},
			},
			Temperature: a.Temperature,
			MaxTokens:   600,
		}

		resp, callErr := a.Client.Complete(ctx, req)
		if callErr != nil {
			err = fmt.Errorf("LLM call attempt %d: %w", attempt+1, callErr)
			continue
		}
		usage.InputTokens += resp.InputTokens
		usage.OutputTokens += resp.OutputTokens

		parsed := parseGuess(resp.Content)
		lastReply = resp.Content

		if parsed != "" && a.Lists.IsValidGuess(parsed) {
			// Valid word found — check if it contradicts hard constraints.
			isInCandidates := false
			for _, c := range candidates {
				if c == parsed {
					isInCandidates = true
					break
				}
			}
			reasoning = extractReasoning(resp.Content)
			if !isInCandidates {
				// Violates hard constraints — fall back but record the violation.
				_ = lastReply
				guess = fallbackCandidate(candidates, a.Lists)
				return guess, reasoning, true, usage, nil
			}
			return parsed, reasoning, false, usage, nil
		}
		// Invalid/unrecognized — retry
	}

	// All attempts exhausted without a valid guess.
	reasoning = extractReasoning(lastReply)
	guess = fallbackCandidate(candidates, a.Lists)
	return guess, reasoning, true, usage, err
}

// parseGuess extracts a 5-letter word from the LLM reply per §9.4:
//  1. Find the last match of "GUESS: <word>" (case-insensitive).
//  2. Else find the last standalone 5-letter alpha token.
//
// Returns lowercase or "" if nothing found.
func parseGuess(reply string) string {
	// Pass 1: last GUESS: line.
	matches := guessLineRe.FindAllStringSubmatch(reply, -1)
	if len(matches) > 0 {
		return strings.ToLower(matches[len(matches)-1][1])
	}

	// Pass 2: last standalone 5-letter alpha token.
	tokens := standalone5Re.FindAllString(reply, -1)
	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		if isAlpha(t) {
			return strings.ToLower(t)
		}
	}
	return ""
}

// isAlpha reports whether every rune in s is a letter.
func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// extractReasoning returns the reply with the last GUESS: line stripped and
// truncated to 2000 characters.
func extractReasoning(reply string) string {
	lines := strings.Split(reply, "\n")
	// Find last GUESS: line and strip it.
	for i := len(lines) - 1; i >= 0; i-- {
		if guessLineRe.MatchString(lines[i]) {
			lines = append(lines[:i], lines[i+1:]...)
			break
		}
	}
	out := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(out) > 2000 {
		out = out[:2000]
	}
	return out
}

// fallbackCandidate returns the first remaining candidate, or a known-valid
// default if the candidate list is unexpectedly empty.
func fallbackCandidate(candidates []string, lists *wordle.WordLists) string {
	if len(candidates) > 0 {
		return candidates[0]
	}
	// Last resort — should never happen in a real game.
	for w := range lists.Guesses {
		return w
	}
	return "crane"
}

// buildUserMessage formats the current board state for the LLM.
func buildUserMessage(g *wordle.Game, candidates []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Guess this 5-letter word. You have %d guess(es) remaining.\n\n",
		g.MaxTurns-len(g.Guesses)))

	if len(g.Guesses) == 0 {
		sb.WriteString("No guesses made yet. This is your first guess.\n")
	} else {
		sb.WriteString("Board so far:\n")
		for i, prev := range g.Guesses {
			sb.WriteString(fmt.Sprintf("  Guess %d: %s → %s\n",
				i+1, strings.ToUpper(prev), g.Feedbacks[i].String()))
		}
		sb.WriteString("\nFeedback key: G=correct position, Y=wrong position, X=not in word\n")

		// Summarize known constraints.
		greens := [wordle.WordLen]byte{}
		yellows := make(map[byte][]int)
		grays := make(map[byte]struct{})

		for i, prev := range g.Guesses {
			for pos := 0; pos < wordle.WordLen; pos++ {
				ch := prev[pos]
				switch g.Feedbacks[i][pos] {
				case wordle.Green:
					greens[pos] = ch
				case wordle.Yellow:
					yellows[ch] = append(yellows[ch], pos+1)
				case wordle.Gray:
					grays[ch] = struct{}{}
				}
			}
		}

		sb.WriteString("\nKnown constraints:\n")
		for pos, ch := range greens {
			if ch != 0 {
				sb.WriteString(fmt.Sprintf("  Position %d is '%c' (green)\n", pos+1, ch))
			}
		}
		for ch, positions := range yellows {
			sb.WriteString(fmt.Sprintf("  '%c' is in the word but not at position(s) %v\n", ch, positions))
		}
		if len(grays) > 0 {
			var grayLetters []byte
			for ch := range grays {
				grayLetters = append(grayLetters, ch)
			}
			sb.WriteString(fmt.Sprintf("  Excluded letters: %s\n", string(grayLetters)))
		}
	}

	sb.WriteString(fmt.Sprintf("\nRemaining candidates: ~%d\n", len(candidates)))
	if len(candidates) <= 5 && len(candidates) > 0 {
		sb.WriteString("Candidates: " + strings.Join(candidates, ", ") + "\n")
	}

	sb.WriteString("\nEnd your reply with:\nGUESS: <WORD>\n")
	return sb.String()
}
