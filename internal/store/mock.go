// Package store contains the Store interface, data structs, and an in-memory
// MockStore for use in tests. MockStore is exported so it can be imported by
// handler tests and other packages.
package store

import (
	"context"
	"sort"
	"sync"
)

// MockStore is a thread-safe in-memory implementation of Store for tests.
// Errors can be injected via the exported Err* fields; set before the call.
type MockStore struct {
	mu sync.Mutex

	runs        map[int64]*Run
	nextRunID   int64
	generations map[int64][]*Generation // keyed by runID
	nextGenID   int64
	games       map[int64][]*Game // keyed by runID
	nextGameID  int64
	guesses     map[int64][]*Guess // keyed by gameID
	nextGuessID int64

	// Error injection — set before the call that should fail.
	ErrMigrate            error
	ErrCreateRun          error
	ErrGetRun             error
	ErrListRuns           error
	ErrUpdateRunStatus    error
	ErrDeleteRun          error
	ErrCreateGeneration   error
	ErrUpdateGenStats     error
	ErrListGenerations    error
	ErrCreateGame         error
	ErrListGames          error
	ErrCreateGuess        error
	ErrListGuesses        error
}

// NewMockStore returns an empty MockStore ready for use.
func NewMockStore() *MockStore {
	return &MockStore{
		runs:        make(map[int64]*Run),
		generations: make(map[int64][]*Generation),
		games:       make(map[int64][]*Game),
		guesses:     make(map[int64][]*Guess),
		nextRunID:   1,
		nextGenID:   1,
		nextGameID:  1,
		nextGuessID: 1,
	}
}

func (m *MockStore) Migrate(_ context.Context) error { return m.ErrMigrate }
func (m *MockStore) Close() error                    { return nil }

// --- runs ---

func (m *MockStore) CreateRun(_ context.Context, r *Run) (int64, error) {
	if m.ErrCreateRun != nil {
		return 0, m.ErrCreateRun
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextRunID
	m.nextRunID++
	rc := *r
	rc.ID = id
	m.runs[id] = &rc
	r.ID = id
	return id, nil
}

func (m *MockStore) GetRun(_ context.Context, id int64) (*Run, error) {
	if m.ErrGetRun != nil {
		return nil, m.ErrGetRun
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return nil, ErrNotFound
	}
	rc := *r
	return &rc, nil
}

// ListRuns returns runs ordered by ID descending (newest first), matching the
// real store's ORDER BY id DESC guarantee.
func (m *MockStore) ListRuns(_ context.Context) ([]*Run, error) {
	if m.ErrListRuns != nil {
		return nil, m.ErrListRuns
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Run, 0, len(m.runs))
	for _, r := range m.runs {
		rc := *r
		out = append(out, &rc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (m *MockStore) UpdateRunStatus(_ context.Context, id int64, status string) error {
	if m.ErrUpdateRunStatus != nil {
		return m.ErrUpdateRunStatus
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	return nil
}

// DeleteRun cascades: removes guesses → games → generations → run, mirroring
// the real store's transactional behaviour.
func (m *MockStore) DeleteRun(_ context.Context, id int64) error {
	if m.ErrDeleteRun != nil {
		return m.ErrDeleteRun
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[id]; !ok {
		return ErrNotFound
	}
	// delete guesses for every game of this run
	for _, g := range m.games[id] {
		delete(m.guesses, g.ID)
	}
	delete(m.games, id)
	delete(m.generations, id)
	delete(m.runs, id)
	return nil
}

// --- generations ---

func (m *MockStore) CreateGeneration(_ context.Context, g *Generation) (int64, error) {
	if m.ErrCreateGeneration != nil {
		return 0, m.ErrCreateGeneration
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextGenID
	m.nextGenID++
	gc := *g
	gc.ID = id
	m.generations[g.RunID] = append(m.generations[g.RunID], &gc)
	g.ID = id
	return id, nil
}

func (m *MockStore) UpdateGenerationStats(_ context.Context, g *Generation) error {
	if m.ErrUpdateGenStats != nil {
		return m.ErrUpdateGenStats
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.generations[g.RunID] {
		if existing.ID == g.ID {
			*existing = *g
			return nil
		}
	}
	return ErrNotFound
}

// ListGenerations returns generations for runID ordered by gen_index ascending.
func (m *MockStore) ListGenerations(_ context.Context, runID int64) ([]*Generation, error) {
	if m.ErrListGenerations != nil {
		return nil, m.ErrListGenerations
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.generations[runID]
	out := make([]*Generation, len(src))
	for i, g := range src {
		gc := *g
		out[i] = &gc
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GenIndex < out[j].GenIndex })
	return out, nil
}

// --- games ---

func (m *MockStore) CreateGame(_ context.Context, g *Game) (int64, error) {
	if m.ErrCreateGame != nil {
		return 0, m.ErrCreateGame
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextGameID
	m.nextGameID++
	gc := *g
	gc.ID = id
	m.games[g.RunID] = append(m.games[g.RunID], &gc)
	g.ID = id
	return id, nil
}

func (m *MockStore) UpdateGame(_ context.Context, g *Game) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, games := range m.games {
		for _, existing := range games {
			if existing.ID == g.ID {
				existing.Won = g.Won
				existing.NumGuesses = g.NumGuesses
				existing.InfoGainTotal = g.InfoGainTotal
				existing.Violations = g.Violations
				return nil
			}
		}
	}
	return ErrNotFound
}

// ListGames returns games for runID, optionally filtered to one genIndex.
func (m *MockStore) ListGames(_ context.Context, runID int64, genIndex *int) ([]*Game, error) {
	if m.ErrListGames != nil {
		return nil, m.ErrListGames
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Game
	for _, g := range m.games[runID] {
		if genIndex != nil && g.GenIndex != *genIndex {
			continue
		}
		gc := *g
		out = append(out, &gc)
	}
	if out == nil {
		out = []*Game{}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// --- guesses ---

func (m *MockStore) CreateGuess(_ context.Context, gu *Guess) (int64, error) {
	if m.ErrCreateGuess != nil {
		return 0, m.ErrCreateGuess
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextGuessID
	m.nextGuessID++
	gc := *gu
	gc.ID = id
	m.guesses[gu.GameID] = append(m.guesses[gu.GameID], &gc)
	gu.ID = id
	return id, nil
}

// ListGuesses returns guesses for gameID ordered by turn_index ascending.
func (m *MockStore) ListGuesses(_ context.Context, gameID int64) ([]*Guess, error) {
	if m.ErrListGuesses != nil {
		return nil, m.ErrListGuesses
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.guesses[gameID]
	out := make([]*Guess, len(src))
	for i, g := range src {
		gc := *g
		out[i] = &gc
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TurnIndex < out[j].TurnIndex })
	return out, nil
}
