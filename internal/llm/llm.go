// Package llm abstracts the LLM chat-completion transport. The concrete
// implementation targets OpenRouter (OpenAI-compatible). See ARCHITECTURE.md §3.
package llm

import "context"

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// CompletionRequest is a chat-completion request.
type CompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// CompletionResponse is the normalized chat-completion result.
type CompletionResponse struct {
	Content      string `json:"content"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// Client performs chat completions. Backend implements an OpenRouter-backed
// client; QA substitutes a scripted mock for tests.
type Client interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
