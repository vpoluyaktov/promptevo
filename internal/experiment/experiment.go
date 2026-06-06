// Package experiment orchestrates a run: it loops generations x games, drives
// the agent and reflector, persists rows, and publishes SSE events through a Hub.
// See ARCHITECTURE.md §3 and §7.
package experiment

import (
	"sync"

	"promptevo/internal/agent"
	"promptevo/internal/reflector"
	"promptevo/internal/store"
	"promptevo/internal/wordle"
)

// DefaultStrategyPrompt is the generation-0 strategy prompt (ARCHITECTURE.md §10).
const DefaultStrategyPrompt = `You are an expert Wordle player. The goal is to find the hidden five-letter
English word in six or fewer guesses.

Rules of the game:
- Every guess must be a valid five-letter English word.
- After each guess you receive feedback for each letter position:
  GREEN  = correct letter in the correct position,
  YELLOW = letter is in the word but in a different position,
  GRAY   = letter is not in the word at all.

Strategy:
- Think step by step. Track which letters are confirmed (green), which are
  present but misplaced (yellow), and which are excluded (gray).
- Never reuse a gray letter, and respect every green position and known
  yellow constraint in your next guess.
- Prefer guesses that test many new common letters early, then converge on
  the answer as constraints accumulate.

Respond with brief reasoning, then end your reply with a line in exactly this
format:
GUESS: <WORD>`

// Event is a single SSE event payload (marshaled to JSON in the "data:" field).
// The Type field selects the variant; see ARCHITECTURE.md §7.
type Event struct {
	Type string `json:"type"`
	// Remaining fields are variant-specific and set by the producer.
	GameID        int64    `json:"gameId,omitempty"`
	RunID         int64    `json:"runId,omitempty"`
	GenIndex      int      `json:"genIndex,omitempty"`
	Turn          int      `json:"turn,omitempty"`
	Guess         string   `json:"guess,omitempty"`
	Feedback      string   `json:"feedback,omitempty"`
	InfoGain      float64  `json:"infoGain,omitempty"`
	Won           *bool    `json:"won,omitempty"`
	NumGuesses    int      `json:"numGuesses,omitempty"`
	Answer        string   `json:"answer,omitempty"`
	SolveRate     *float64 `json:"solveRate,omitempty"`
	MeanGuesses   *float64 `json:"meanGuesses,omitempty"`
	MeanInfoGain  *float64 `json:"meanInfoGain,omitempty"`
	ViolationRate *float64 `json:"violationRate,omitempty"`
	Prompt        string   `json:"prompt,omitempty"`
	Status        string   `json:"status,omitempty"`
	Convergence   string   `json:"convergence,omitempty"`
	Message       string   `json:"message,omitempty"`
}

// Hub fans out SSE events to subscribers keyed by runID.
// TODO(backend): implement subscribe/publish/unsubscribe with channels.
type Hub struct {
	mu sync.Mutex
	// subscribers[runID] -> set of event channels
	subscribers map[int64]map[chan Event]struct{}
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{subscribers: map[int64]map[chan Event]struct{}{}}
}

// Subscribe returns a channel receiving events for runID and an unsubscribe func.
// TODO(backend): implement.
func (h *Hub) Subscribe(runID int64) (<-chan Event, func()) { return nil, func() {} }

// Publish delivers ev to all subscribers of its run.
// TODO(backend): implement.
func (h *Hub) Publish(runID int64, ev Event) {}

// Orchestrator runs experiments.
type Orchestrator struct {
	Store     store.Store
	Agent     *agent.Agent
	Reflector *reflector.Reflector
	Lists     *wordle.WordLists
	Hub       *Hub
}

// StartRun launches the run goroutine for runID (ARCHITECTURE.md §3).
// TODO(backend): implement the generation/game loop.
func (o *Orchestrator) StartRun(runID int64) {}

// Convergence classifies a run from its completed generations' solve rates.
// Returns "improving" | "oscillating" | "stable" per ARCHITECTURE.md §9.6.
// TODO(backend): implement.
func Convergence(solveRates []float64) string { return "improving" }
