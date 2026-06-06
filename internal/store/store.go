// Package store is the persistence layer (SQLite via modernc.org/sqlite).
// The Store interface enables dependency injection and test mocking.
// See ARCHITECTURE.md §5–§6.
package store

import (
	"context"
	"errors"

	// Registers the pure-Go, CGO-free "sqlite" driver for database/sql.
	// The sqliteStore implementation (Backend) opens with sql.Open("sqlite", DB_PATH).
	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Run mirrors the runs table. JSON tags match the API wire format (§4).
type Run struct {
	ID             int64   `json:"id"`
	CreatedAt      string  `json:"createdAt"`
	PlayerModel    string  `json:"playerModel"`
	ReflectorModel string  `json:"reflectorModel"`
	Temperature    float64 `json:"temperature"`
	Seed           int64   `json:"seed"`
	Generations    int     `json:"generations"`
	GamesPerGen    int     `json:"gamesPerGen"`
	WordSampleSize int     `json:"wordSampleSize"`
	Status         string  `json:"status"`
	ConfigJSON     string  `json:"-"`
}

// Generation mirrors the generations table. Nullable stats use pointers so
// "not yet computed" is distinct from zero.
type Generation struct {
	ID             int64    `json:"-"`
	RunID          int64    `json:"-"`
	GenIndex       int      `json:"genIndex"`
	PromptText     string   `json:"promptText"`
	PromptLen      int      `json:"promptLen"`
	ReflectionText *string  `json:"reflectionText"`
	SolveRate      *float64 `json:"solveRate"`
	MeanGuesses    *float64 `json:"meanGuesses"`
	MeanInfoGain   *float64 `json:"meanInfoGain"`
	ViolationRate  *float64 `json:"violationRate"`
	TokensUsed     int      `json:"tokensUsed"`
}

// Game mirrors the games table.
type Game struct {
	ID            int64   `json:"id"`
	RunID         int64   `json:"-"`
	GenIndex      int     `json:"genIndex"`
	Answer        string  `json:"answer"`
	Won           bool    `json:"won"`
	NumGuesses    int     `json:"numGuesses"`
	InfoGainTotal float64 `json:"infoGainTotal"`
	Violations    int     `json:"violations"`
	AgentType     string  `json:"agentType"`
}

// Guess mirrors the guesses table.
type Guess struct {
	ID            int64   `json:"id"`
	GameID        int64   `json:"-"`
	TurnIndex     int     `json:"turnIndex"`
	Guess         string  `json:"guess"`
	Feedback      string  `json:"feedback"`
	InfoGainBits  float64 `json:"infoGainBits"`
	ReasoningText *string `json:"reasoningText"`
}

// Store is the full persistence contract used by handlers and the orchestrator.
type Store interface {
	// lifecycle
	Migrate(ctx context.Context) error
	Close() error

	// runs
	CreateRun(ctx context.Context, r *Run) (int64, error)
	GetRun(ctx context.Context, id int64) (*Run, error)
	ListRuns(ctx context.Context) ([]*Run, error)
	UpdateRunStatus(ctx context.Context, id int64, status string) error
	DeleteRun(ctx context.Context, id int64) error // tx: guesses -> games -> generations -> run

	// generations
	CreateGeneration(ctx context.Context, g *Generation) (int64, error)
	UpdateGenerationStats(ctx context.Context, g *Generation) error
	ListGenerations(ctx context.Context, runID int64) ([]*Generation, error)

	// games
	CreateGame(ctx context.Context, g *Game) (int64, error)
	ListGames(ctx context.Context, runID int64, genIndex *int) ([]*Game, error)

	// guesses
	CreateGuess(ctx context.Context, gu *Guess) (int64, error)
	ListGuesses(ctx context.Context, gameID int64) ([]*Guess, error)
}
