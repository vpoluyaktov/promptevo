// Package llm provides the LLM client interface and an OpenRouter implementation.
// MockClient is exported here so it can be imported by any package's tests.
package llm

import (
	"context"
	"errors"
	"sync"
)

// MockResponse is a single scripted response for MockClient.
type MockResponse struct {
	Resp CompletionResponse
	Err  error
}

// MockClient is a scripted Client for use in tests. It records every Complete
// call and returns pre-configured MockResponse values in FIFO order. When
// responses are exhausted it returns an error so tests that call Complete more
// times than expected fail loudly.
type MockClient struct {
	mu        sync.Mutex
	Calls     []CompletionRequest // recorded in call order
	responses []MockResponse
}

// NewMockClient creates a MockClient that will return the given responses in
// the order they are provided.
func NewMockClient(responses ...MockResponse) *MockClient {
	return &MockClient{responses: responses}
}

// Complete records the call and returns the next configured response.
func (m *MockClient) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, req)
	if len(m.responses) == 0 {
		return CompletionResponse{}, errors.New("mock: no more responses configured")
	}
	r := m.responses[0]
	m.responses = m.responses[1:]
	return r.Resp, r.Err
}

// CallCount returns the number of Complete calls recorded so far.
func (m *MockClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}
