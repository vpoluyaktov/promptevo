// Package llm abstracts the LLM chat-completion transport. The concrete
// implementation targets OpenRouter (OpenAI-compatible). See ARCHITECTURE.md §3.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

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

// ─── OpenRouter client ───────────────────────────────────────────────────────

// openRouterResponse is the OpenAI-compatible wire format returned by OpenRouter.
type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// OpenRouterClient implements Client against OpenRouter's OpenAI-compatible API.
type OpenRouterClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewOpenRouterClient returns a client that posts to baseURL (typically
// "https://openrouter.ai/api/v1") with the given API key and request timeout.
// Pass 0 for timeout to use the default (120 s).
func NewOpenRouterClient(apiKey, baseURL string, timeout time.Duration) *OpenRouterClient {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &OpenRouterClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// Complete sends a chat completion request to OpenRouter, retrying transient
// 429/5xx errors with exponential backoff (up to 3 attempts).
func (c *OpenRouterClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return CompletionResponse{}, ctx.Err()
			case <-time.After(wait):
			}
		}

		resp, err := c.doRequest(ctx, url, body)
		if err != nil {
			lastErr = err
			// Only retry on transient network errors
			continue
		}

		// Retry on 429 and 5xx
		if resp.statusCode == http.StatusTooManyRequests || resp.statusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.statusCode, resp.body)
			continue
		}

		if resp.statusCode != http.StatusOK {
			return CompletionResponse{}, fmt.Errorf("HTTP %d: %s", resp.statusCode, resp.body)
		}

		var or openRouterResponse
		if err := json.Unmarshal(resp.body, &or); err != nil {
			return CompletionResponse{}, fmt.Errorf("decode response: %w", err)
		}
		if or.Error != nil {
			return CompletionResponse{}, fmt.Errorf("API error %d: %s", or.Error.Code, or.Error.Message)
		}
		if len(or.Choices) == 0 {
			return CompletionResponse{}, errors.New("no choices in response")
		}

		return CompletionResponse{
			Content:      or.Choices[0].Message.Content,
			InputTokens:  or.Usage.PromptTokens,
			OutputTokens: or.Usage.CompletionTokens,
		}, nil
	}

	return CompletionResponse{}, fmt.Errorf("after 3 attempts: %w", lastErr)
}

type httpResult struct {
	statusCode int
	body       []byte
}

func (c *OpenRouterClient) doRequest(ctx context.Context, url string, body []byte) (httpResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return httpResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/promptevo")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return httpResult{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return httpResult{}, fmt.Errorf("read response body: %w", err)
	}

	return httpResult{statusCode: resp.StatusCode, body: respBody}, nil
}

// ─── OpenAI client ───────────────────────────────────────────────────────────

// OpenAIClient implements Client against OpenAI's chat-completions API.
// The response wire format is identical to OpenRouter, so openRouterResponse is reused.
type OpenAIClient struct {
	apiKey string
	http   *http.Client
}

// NewOpenAIClient returns a client targeting api.openai.com with the given API
// key and request timeout. Pass 0 for timeout to use the default (120 s).
func NewOpenAIClient(apiKey string, timeout time.Duration) *OpenAIClient {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &OpenAIClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: timeout},
	}
}

// Complete sends a chat-completion request to OpenAI, retrying transient
// 429/5xx errors with exponential backoff (up to 3 attempts).
func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	const endpoint = "https://api.openai.com/v1/chat/completions"

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return CompletionResponse{}, ctx.Err()
			case <-time.After(wait):
			}
		}

		resp, err := c.doRequest(ctx, endpoint, body)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.statusCode == http.StatusTooManyRequests || resp.statusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.statusCode, resp.body)
			continue
		}

		if resp.statusCode != http.StatusOK {
			return CompletionResponse{}, fmt.Errorf("HTTP %d: %s", resp.statusCode, resp.body)
		}

		var or openRouterResponse
		if err := json.Unmarshal(resp.body, &or); err != nil {
			return CompletionResponse{}, fmt.Errorf("decode response: %w", err)
		}
		if or.Error != nil {
			return CompletionResponse{}, fmt.Errorf("API error %d: %s", or.Error.Code, or.Error.Message)
		}
		if len(or.Choices) == 0 {
			return CompletionResponse{}, errors.New("no choices in response")
		}

		return CompletionResponse{
			Content:      or.Choices[0].Message.Content,
			InputTokens:  or.Usage.PromptTokens,
			OutputTokens: or.Usage.CompletionTokens,
		}, nil
	}

	return CompletionResponse{}, fmt.Errorf("after 3 attempts: %w", lastErr)
}

func (c *OpenAIClient) doRequest(ctx context.Context, url string, body []byte) (httpResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return httpResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return httpResult{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return httpResult{}, fmt.Errorf("read response body: %w", err)
	}

	return httpResult{statusCode: resp.StatusCode, body: respBody}, nil
}

// ─── Anthropic client ────────────────────────────────────────────────────────

// anthropicRequest is the wire format for Anthropic's Messages API.
type anthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

// anthropicResponse is the successful wire format from Anthropic's Messages API.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// anthropicErrorResponse is the error body returned by Anthropic on non-2xx.
type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// AnthropicClient implements Client against Anthropic's Messages API.
type AnthropicClient struct {
	apiKey string
	http   *http.Client
}

// NewAnthropicClient returns a client targeting api.anthropic.com with the
// given API key and request timeout. Pass 0 for timeout to use the default (120 s).
func NewAnthropicClient(apiKey string, timeout time.Duration) *AnthropicClient {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &AnthropicClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: timeout},
	}
}

// Complete sends a chat-completion request to Anthropic, retrying transient
// 429/529 errors with exponential backoff (up to 3 attempts).
// System messages are extracted from req.Messages and placed in the top-level
// "system" field as required by the Anthropic Messages API.
func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	// Anthropic requires system content in a top-level field, not in messages[].
	var systemText string
	var msgs []Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemText = m.Content
		} else {
			msgs = append(msgs, m)
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}

	reqBody := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    systemText,
		Messages:  msgs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	const endpoint = "https://api.anthropic.com/v1/messages"

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return CompletionResponse{}, ctx.Err()
			case <-time.After(wait):
			}
		}

		resp, err := c.doRequest(ctx, endpoint, body)
		if err != nil {
			lastErr = err
			continue
		}

		// Retry on 429 (rate limit) and 529 (Anthropic overload).
		if resp.statusCode == http.StatusTooManyRequests || resp.statusCode == 529 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.statusCode, resp.body)
			continue
		}

		if resp.statusCode != http.StatusOK {
			var errResp anthropicErrorResponse
			if jsonErr := json.Unmarshal(resp.body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
				return CompletionResponse{}, fmt.Errorf("API error (%s): %s", errResp.Error.Type, errResp.Error.Message)
			}
			return CompletionResponse{}, fmt.Errorf("HTTP %d: %s", resp.statusCode, resp.body)
		}

		var respData anthropicResponse
		if err := json.Unmarshal(resp.body, &respData); err != nil {
			return CompletionResponse{}, fmt.Errorf("decode response: %w", err)
		}
		if len(respData.Content) == 0 {
			return CompletionResponse{}, errors.New("no content in response")
		}

		return CompletionResponse{
			Content:      respData.Content[0].Text,
			InputTokens:  respData.Usage.InputTokens,
			OutputTokens: respData.Usage.OutputTokens,
		}, nil
	}

	return CompletionResponse{}, fmt.Errorf("after 3 attempts: %w", lastErr)
}

func (c *AnthropicClient) doRequest(ctx context.Context, url string, body []byte) (httpResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return httpResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return httpResult{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return httpResult{}, fmt.Errorf("read response body: %w", err)
	}

	return httpResult{statusCode: resp.StatusCode, body: respBody}, nil
}

// ─── Factory ─────────────────────────────────────────────────────────────────

// NewClientForProvider returns a Client for the given provider name.
// provider must be one of: "openrouter", "anthropic", "openai".
func NewClientForProvider(provider, apiKey string, timeout time.Duration) (Client, error) {
	switch provider {
	case "openrouter":
		return NewOpenRouterClient(apiKey, "https://openrouter.ai/api/v1", timeout), nil
	case "openai":
		return NewOpenAIClient(apiKey, timeout), nil
	case "anthropic":
		return NewAnthropicClient(apiKey, timeout), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", provider)
	}
}

