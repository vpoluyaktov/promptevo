// Package reflector_test verifies the prompt-parser (ParsePrompt) and the
// Reflector.Reflect method using a scripted MockClient from the llm package.
package reflector_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"promptevo/internal/llm"
	"promptevo/internal/reflector"
)

// validPrompt is a prompt string long enough to pass the 50-char minimum and
// short enough to stay under the 4000-char maximum (if those bounds are
// enforced by ParsePrompt per the QA task specification).
const validPrompt = "You are an expert Wordle player. Think step by step and track all constraints."

// wrapPrompt wraps text in the required delimiters.
func wrapPrompt(text string) string {
	return reflector.PromptStartDelimiter + "\n" + text + "\n" + reflector.PromptEndDelimiter
}

// --- ParsePrompt table-driven tests ---

func TestParsePrompt(t *testing.T) {
	// A prompt that is definitely > 50 chars and < 4000 chars.
	longEnough := strings.Repeat("x", 60)
	// A prompt over the 4000-char limit (if enforced).
	tooLong := strings.Repeat("x", 4001)

	tests := []struct {
		name    string
		raw     string
		wantOK  bool
		wantOut string // non-empty only when wantOK == true
	}{
		// ── valid cases ────────────────────────────────────────────────────
		{
			name:    "valid_wrapped_prompt",
			raw:     wrapPrompt(validPrompt),
			wantOK:  true,
			wantOut: validPrompt,
		},
		{
			// Leading/trailing whitespace inside the block must be trimmed.
			name:    "trims_inner_whitespace",
			raw:     reflector.PromptStartDelimiter + "\n   " + validPrompt + "   \n" + reflector.PromptEndDelimiter,
			wantOK:  true,
			wantOut: validPrompt,
		},
		{
			// First well-formed block wins when multiple pairs appear.
			name:    "first_block_wins",
			raw:     wrapPrompt(validPrompt) + "\n" + wrapPrompt("second prompt content here that is also long enough"),
			wantOK:  true,
			wantOut: validPrompt,
		},
		{
			// Surrounding text outside the delimiters is ignored.
			name:    "surrounding_text_ignored",
			raw:     "Here is my analysis...\n" + wrapPrompt(validPrompt) + "\nEnd of response.",
			wantOK:  true,
			wantOut: validPrompt,
		},

		// ── missing / reversed delimiters ─────────────────────────────────
		{
			name:   "missing_start_delimiter",
			raw:    validPrompt + "\n" + reflector.PromptEndDelimiter,
			wantOK: false,
		},
		{
			name:   "missing_end_delimiter",
			raw:    reflector.PromptStartDelimiter + "\n" + validPrompt,
			wantOK: false,
		},
		{
			name:   "no_delimiters_at_all",
			raw:    validPrompt,
			wantOK: false,
		},
		{
			// START appears after END → malformed.
			name:   "reversed_delimiters",
			raw:    reflector.PromptEndDelimiter + "\n" + validPrompt + "\n" + reflector.PromptStartDelimiter,
			wantOK: false,
		},
		{
			name:   "empty_raw_string",
			raw:    "",
			wantOK: false,
		},

		// ── empty or too-short content between delimiters ─────────────────
		{
			// Nothing between the delimiters.
			name:   "empty_content_between_delimiters",
			raw:    reflector.PromptStartDelimiter + "\n" + reflector.PromptEndDelimiter,
			wantOK: false,
		},
		{
			// Content that is whitespace-only (trims to empty).
			name:   "whitespace_only_content",
			raw:    reflector.PromptStartDelimiter + "\n   \n\t\n" + reflector.PromptEndDelimiter,
			wantOK: false,
		},
		{
			// Content shorter than 50 chars (QA task spec constraint).
			name:   "content_under_50_chars",
			raw:    wrapPrompt("too short"),
			wantOK: false,
		},

		// ── content over 4000-char limit (QA task spec constraint) ────────
		{
			name:   "content_over_4000_chars",
			raw:    wrapPrompt(tooLong),
			wantOK: false,
		},

		// ── edge: exactly at length boundaries ────────────────────────────
		{
			// Exactly 50 chars should be accepted (boundary inclusive).
			name:    "exactly_50_chars",
			raw:     wrapPrompt(strings.Repeat("a", 50)),
			wantOK:  true,
			wantOut: strings.Repeat("a", 50),
		},
		{
			// Exactly 4000 chars should be accepted (boundary inclusive).
			name:    "exactly_4000_chars",
			raw:     wrapPrompt(longEnough + strings.Repeat("b", 4000-len(longEnough))),
			wantOK:  true,
			wantOut: longEnough + strings.Repeat("b", 4000-len(longEnough)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := reflector.ParsePrompt(tc.raw)
			if ok != tc.wantOK {
				t.Errorf("ParsePrompt ok = %v, want %v (got prompt: %q)", ok, tc.wantOK, got)
				return
			}
			if tc.wantOK && got != tc.wantOut {
				t.Errorf("ParsePrompt prompt =\n%q\nwant\n%q", got, tc.wantOut)
			}
			if !tc.wantOK && got != "" {
				t.Errorf("ParsePrompt: ok=false but returned non-empty prompt %q", got)
			}
		})
	}
}

// --- Reflector.Reflect with MockClient ---

func TestReflectorReflect_ValidResponse(t *testing.T) {
	newPromptText := validPrompt + " additionally use yellow letter positions to narrow candidates."
	mockResp := wrapPrompt(newPromptText)

	client := llm.NewMockClient(llm.MockResponse{
		Resp: llm.CompletionResponse{
			Content:      mockResp,
			InputTokens:  100,
			OutputTokens: 50,
		},
	})

	r := &reflector.Reflector{
		Client:      client,
		Model:       "openai/gpt-4o-mini",
		Temperature: 0.7,
	}

	currentPrompt := validPrompt
	stats := reflector.GenerationStats{
		GenIndex:      0,
		SolveRate:     0.6,
		MeanGuesses:   4.3,
		MeanInfoGain:  6.81,
		ViolationRate: 0.08,
	}

	newPrompt, _, _, ok, usage, err := r.Reflect(context.Background(), currentPrompt, stats)
	if err != nil {
		t.Fatalf("Reflect returned unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("Reflect ok = false, want true (valid delimited response)")
	}
	if newPrompt != newPromptText {
		t.Errorf("Reflect newPrompt =\n%q\nwant\n%q", newPrompt, newPromptText)
	}
	if usage.InputTokens != 100 || usage.OutputTokens != 50 {
		t.Errorf("Reflect usage = %+v, want InputTokens=100 OutputTokens=50", usage)
	}
	if client.CallCount() != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.CallCount())
	}
}

func TestReflectorReflect_MalformedResponse_FallsBack(t *testing.T) {
	// Missing delimiters → ParsePrompt returns ok=false → Reflect falls back.
	client := llm.NewMockClient(llm.MockResponse{
		Resp: llm.CompletionResponse{
			Content: "Here is my analysis without any delimiter block.",
		},
	})

	r := &reflector.Reflector{
		Client:      client,
		Model:       "openai/gpt-4o-mini",
		Temperature: 0.7,
	}

	currentPrompt := validPrompt
	newPrompt, _, _, ok, _, err := r.Reflect(context.Background(), currentPrompt, reflector.GenerationStats{})
	if err != nil {
		t.Fatalf("Reflect returned unexpected error: %v", err)
	}
	if ok {
		t.Error("Reflect ok = true, want false (malformed response)")
	}
	// Caller must reuse the current prompt.
	if newPrompt != currentPrompt {
		t.Errorf("Reflect fallback: newPrompt = %q, want currentPrompt %q", newPrompt, currentPrompt)
	}
}

func TestReflectorReflect_EmptyResponse_FallsBack(t *testing.T) {
	client := llm.NewMockClient(llm.MockResponse{
		Resp: llm.CompletionResponse{Content: ""},
	})

	r := &reflector.Reflector{
		Client:      client,
		Model:       "openai/gpt-4o-mini",
		Temperature: 0.7,
	}

	currentPrompt := validPrompt
	newPrompt, _, _, ok, _, err := r.Reflect(context.Background(), currentPrompt, reflector.GenerationStats{})
	if err != nil {
		t.Fatalf("Reflect returned unexpected error: %v", err)
	}
	if ok {
		t.Error("Reflect ok = true, want false (empty LLM response)")
	}
	if newPrompt != currentPrompt {
		t.Errorf("Reflect fallback: newPrompt = %q, want currentPrompt %q", newPrompt, currentPrompt)
	}
}

func TestReflectorReflect_LLMError_ReturnsError(t *testing.T) {
	injectErr := errors.New("openrouter: rate limited")
	client := llm.NewMockClient(llm.MockResponse{Err: injectErr})

	r := &reflector.Reflector{
		Client:      client,
		Model:       "openai/gpt-4o-mini",
		Temperature: 0.7,
	}

	_, _, _, _, _, err := r.Reflect(context.Background(), validPrompt, reflector.GenerationStats{})
	if err == nil {
		t.Error("Reflect: expected error from LLM, got nil")
	}
}

func TestReflectorReflect_CallsLLMWithCurrentPromptInMessages(t *testing.T) {
	// Verify that the currentPrompt is included in the messages sent to the LLM.
	newPromptText := strings.Repeat("improved strategy prompt content here", 3) // > 50 chars
	client := llm.NewMockClient(llm.MockResponse{
		Resp: llm.CompletionResponse{Content: wrapPrompt(newPromptText)},
	})

	r := &reflector.Reflector{
		Client:      client,
		Model:       "openai/gpt-4o-mini",
		Temperature: 0.7,
	}

	currentPrompt := validPrompt
	_, _, _, _, _, _ = r.Reflect(context.Background(), currentPrompt, reflector.GenerationStats{
		SolveRate: 0.4,
	})

	if client.CallCount() == 0 {
		t.Fatal("Reflect made no LLM calls")
	}

	// The currentPrompt must appear somewhere in the messages sent.
	found := false
	for _, msg := range client.Calls[0].Messages {
		if strings.Contains(msg.Content, currentPrompt) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("currentPrompt not found in any LLM message; messages: %+v", client.Calls[0].Messages)
	}
}
