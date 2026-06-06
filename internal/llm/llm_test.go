// Package llm_test verifies the MockClient used in other packages' tests.
package llm_test

import (
	"context"
	"errors"
	"testing"

	"promptevo/internal/llm"
)

func TestMockClient_ReturnsResponsesInOrder(t *testing.T) {
	r1 := llm.CompletionResponse{Content: "first", InputTokens: 10, OutputTokens: 5}
	r2 := llm.CompletionResponse{Content: "second", InputTokens: 20, OutputTokens: 8}

	client := llm.NewMockClient(
		llm.MockResponse{Resp: r1},
		llm.MockResponse{Resp: r2},
	)

	req := llm.CompletionRequest{
		Model:       "openai/gpt-4o-mini",
		Messages:    []llm.Message{{Role: "user", Content: "hello"}},
		Temperature: 0.7,
	}

	got1, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("call 1: unexpected error: %v", err)
	}
	if got1 != r1 {
		t.Errorf("call 1: got %+v, want %+v", got1, r1)
	}

	got2, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("call 2: unexpected error: %v", err)
	}
	if got2 != r2 {
		t.Errorf("call 2: got %+v, want %+v", got2, r2)
	}
}

func TestMockClient_RecordsCalls(t *testing.T) {
	client := llm.NewMockClient(
		llm.MockResponse{Resp: llm.CompletionResponse{Content: "ok"}},
		llm.MockResponse{Resp: llm.CompletionResponse{Content: "ok2"}},
	)

	req1 := llm.CompletionRequest{
		Model:    "model-a",
		Messages: []llm.Message{{Role: "system", Content: "be helpful"}},
	}
	req2 := llm.CompletionRequest{
		Model:    "model-b",
		Messages: []llm.Message{{Role: "user", Content: "guess: CRANE"}},
	}

	_, _ = client.Complete(context.Background(), req1)
	_, _ = client.Complete(context.Background(), req2)

	if client.CallCount() != 2 {
		t.Fatalf("CallCount = %d, want 2", client.CallCount())
	}
	if client.Calls[0].Model != "model-a" {
		t.Errorf("Calls[0].Model = %q, want model-a", client.Calls[0].Model)
	}
	if client.Calls[1].Model != "model-b" {
		t.Errorf("Calls[1].Model = %q, want model-b", client.Calls[1].Model)
	}
}

func TestMockClient_ReturnsConfiguredError(t *testing.T) {
	sentinel := errors.New("rate limit")
	client := llm.NewMockClient(llm.MockResponse{Err: sentinel})

	_, err := client.Complete(context.Background(), llm.CompletionRequest{})
	if !errors.Is(err, sentinel) {
		t.Errorf("Complete: got error %v, want %v", err, sentinel)
	}
	// Call was still recorded even though it errored.
	if client.CallCount() != 1 {
		t.Errorf("CallCount = %d, want 1", client.CallCount())
	}
}

func TestMockClient_ExhaustedResponses_ReturnsError(t *testing.T) {
	// No responses configured at all.
	client := llm.NewMockClient()

	_, err := client.Complete(context.Background(), llm.CompletionRequest{})
	if err == nil {
		t.Error("Complete: expected error when no responses remain, got nil")
	}
}

func TestMockClient_MixedSuccessAndError(t *testing.T) {
	sentinel := errors.New("timeout")
	client := llm.NewMockClient(
		llm.MockResponse{Resp: llm.CompletionResponse{Content: "ok"}},
		llm.MockResponse{Err: sentinel},
		llm.MockResponse{Resp: llm.CompletionResponse{Content: "recovered"}},
	)

	resp1, err1 := client.Complete(context.Background(), llm.CompletionRequest{})
	if err1 != nil || resp1.Content != "ok" {
		t.Errorf("call 1: got resp=%+v err=%v, want content=ok err=nil", resp1, err1)
	}

	_, err2 := client.Complete(context.Background(), llm.CompletionRequest{})
	if !errors.Is(err2, sentinel) {
		t.Errorf("call 2: got err=%v, want %v", err2, sentinel)
	}

	resp3, err3 := client.Complete(context.Background(), llm.CompletionRequest{})
	if err3 != nil || resp3.Content != "recovered" {
		t.Errorf("call 3: got resp=%+v err=%v, want content=recovered err=nil", resp3, err3)
	}

	if client.CallCount() != 3 {
		t.Errorf("CallCount = %d, want 3", client.CallCount())
	}
}

func TestMockClient_ZeroValueRequest_IsRecorded(t *testing.T) {
	client := llm.NewMockClient(llm.MockResponse{Resp: llm.CompletionResponse{}})

	_, _ = client.Complete(context.Background(), llm.CompletionRequest{})

	if len(client.Calls) != 1 {
		t.Fatalf("len(Calls) = %d, want 1", len(client.Calls))
	}
}

func TestMockClient_ContextCancelled_IsStillRecorded(t *testing.T) {
	// The mock ignores the context but should still record the call.
	client := llm.NewMockClient(llm.MockResponse{Resp: llm.CompletionResponse{Content: "hi"}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, _ = client.Complete(ctx, llm.CompletionRequest{})
	if client.CallCount() != 1 {
		t.Errorf("CallCount = %d after cancelled-context call, want 1", client.CallCount())
	}
}
