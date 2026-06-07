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
	ErrMigrate                  error
	ErrCreateRun                error
	ErrGetRun                   error
	ErrListRuns                 error
	ErrUpdateRunStatus          error
	ErrDeleteRun                error
	ErrCreateGeneration         error
	ErrUpdateGenStats           error
	ErrListGenerations          error
	ErrCreateGame               error
	ErrListGames                error
	ErrCreateGuess              error
	ErrListGuesses              error
	ErrGameOutcomeCounts        error
	ErrTurnInfoGainStats        error
	ErrOpeningWordCounts        error
	ErrReasoningVerbosityStats  error
	ErrWordDifficultyStats      error
	ErrBaselineOutcomeCounts    error
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

// --- analytics ---

// GameOutcomeCounts aggregates game outcomes for LLM games in-memory, mirroring
// the SQL GROUP BY gen_index, won, num_guesses.
func (m *MockStore) GameOutcomeCounts(_ context.Context, runID int64) ([]OutcomeCount, error) {
	if m.ErrGameOutcomeCounts != nil {
		return nil, m.ErrGameOutcomeCounts
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	type key struct {
		GenIndex   int
		Won        bool
		NumGuesses int
	}
	counts := make(map[key]int)
	for _, g := range m.games[runID] {
		if g.AgentType != "llm" {
			continue
		}
		k := key{g.GenIndex, g.Won, g.NumGuesses}
		counts[k]++
	}

	out := make([]OutcomeCount, 0, len(counts))
	for k, cnt := range counts {
		out = append(out, OutcomeCount{
			GenIndex:   k.GenIndex,
			Won:        k.Won,
			NumGuesses: k.NumGuesses,
			Count:      cnt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GenIndex != out[j].GenIndex {
			return out[i].GenIndex < out[j].GenIndex
		}
		wi, wj := 0, 0
		if out[i].Won {
			wi = 1
		}
		if out[j].Won {
			wj = 1
		}
		if wi != wj {
			return wi < wj
		}
		return out[i].NumGuesses < out[j].NumGuesses
	})
	return out, nil
}

// TurnInfoGainStats computes mean info gain per (gen, turn) for LLM games.
func (m *MockStore) TurnInfoGainStats(_ context.Context, runID int64) ([]TurnInfoGainStat, error) {
	if m.ErrTurnInfoGainStats != nil {
		return nil, m.ErrTurnInfoGainStats
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	type key struct {
		GenIndex  int
		TurnIndex int
	}
	type acc struct {
		sum float64
		n   int
	}
	accs := make(map[key]*acc)

	for _, g := range m.games[runID] {
		if g.AgentType != "llm" {
			continue
		}
		for _, gu := range m.guesses[g.ID] {
			k := key{g.GenIndex, gu.TurnIndex}
			if accs[k] == nil {
				accs[k] = &acc{}
			}
			accs[k].sum += gu.InfoGainBits
			accs[k].n++
		}
	}

	out := make([]TurnInfoGainStat, 0, len(accs))
	for k, a := range accs {
		out = append(out, TurnInfoGainStat{
			GenIndex:     k.GenIndex,
			TurnIndex:    k.TurnIndex,
			MeanInfoGain: a.sum / float64(a.n),
			N:            a.n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GenIndex != out[j].GenIndex {
			return out[i].GenIndex < out[j].GenIndex
		}
		return out[i].TurnIndex < out[j].TurnIndex
	})
	return out, nil
}

// OpeningWordCounts counts first-turn guesses per gen for LLM games.
func (m *MockStore) OpeningWordCounts(_ context.Context, runID int64) ([]OpeningWordCount, error) {
	if m.ErrOpeningWordCounts != nil {
		return nil, m.ErrOpeningWordCounts
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	type key struct {
		GenIndex int
		Guess    string
	}
	counts := make(map[key]int)

	for _, g := range m.games[runID] {
		if g.AgentType != "llm" {
			continue
		}
		for _, gu := range m.guesses[g.ID] {
			if gu.TurnIndex == 0 {
				k := key{g.GenIndex, gu.Guess}
				counts[k]++
			}
		}
	}

	out := make([]OpeningWordCount, 0, len(counts))
	for k, cnt := range counts {
		out = append(out, OpeningWordCount{
			GenIndex: k.GenIndex,
			Guess:    k.Guess,
			Count:    cnt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GenIndex != out[j].GenIndex {
			return out[i].GenIndex < out[j].GenIndex
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count // descending count
		}
		return out[i].Guess < out[j].Guess
	})
	return out, nil
}

// ReasoningVerbosityStats computes total reasoning chars per game for LLM games.
func (m *MockStore) ReasoningVerbosityStats(_ context.Context, runID int64) ([]ReasoningStat, error) {
	if m.ErrReasoningVerbosityStats != nil {
		return nil, m.ErrReasoningVerbosityStats
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	type gameKey struct {
		id       int64
		genIndex int
	}
	var gameList []gameKey
	gameMap := make(map[int64]*Game)

	for _, g := range m.games[runID] {
		if g.AgentType != "llm" {
			continue
		}
		gameList = append(gameList, gameKey{g.ID, g.GenIndex})
		gameMap[g.ID] = g
	}
	sort.Slice(gameList, func(i, j int) bool {
		if gameList[i].genIndex != gameList[j].genIndex {
			return gameList[i].genIndex < gameList[j].genIndex
		}
		return gameList[i].id < gameList[j].id
	})

	out := make([]ReasoningStat, 0, len(gameList))
	for _, gk := range gameList {
		g := gameMap[gk.id]
		chars := 0
		for _, gu := range m.guesses[g.ID] {
			if gu.ReasoningText != nil {
				chars += len([]rune(*gu.ReasoningText))
			}
		}
		out = append(out, ReasoningStat{
			GameID:         g.ID,
			GenIndex:       g.GenIndex,
			Won:            g.Won,
			ReasoningChars: chars,
			NumGuesses:     g.NumGuesses,
		})
	}
	return out, nil
}

// WordDifficultyStats computes per-answer win rates for LLM games, sorted
// hardest-first (ascending win rate, then ascending answer).
func (m *MockStore) WordDifficultyStats(_ context.Context, runID int64) ([]WordDifficultyStat, error) {
	if m.ErrWordDifficultyStats != nil {
		return nil, m.ErrWordDifficultyStats
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	type acc struct {
		games int
		wins  int
	}
	accs := make(map[string]*acc)

	for _, g := range m.games[runID] {
		if g.AgentType != "llm" {
			continue
		}
		if accs[g.Answer] == nil {
			accs[g.Answer] = &acc{}
		}
		accs[g.Answer].games++
		if g.Won {
			accs[g.Answer].wins++
		}
	}

	out := make([]WordDifficultyStat, 0, len(accs))
	for answer, a := range accs {
		out = append(out, WordDifficultyStat{
			Answer: answer,
			Games:  a.games,
			Wins:   a.wins,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ri := float64(out[i].Wins) / float64(out[i].Games)
		rj := float64(out[j].Wins) / float64(out[j].Games)
		if ri != rj {
			return ri < rj
		}
		return out[i].Answer < out[j].Answer
	})
	return out, nil
}

// BaselineOutcomeCounts returns solve stats for non-llm agents.
func (m *MockStore) BaselineOutcomeCounts(_ context.Context, runID int64) ([]BaselineSolveStat, error) {
	if m.ErrBaselineOutcomeCounts != nil {
		return nil, m.ErrBaselineOutcomeCounts
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	type acc struct {
		total int
		wins  int
	}
	accs := make(map[string]*acc)
	for _, g := range m.games[runID] {
		if g.AgentType == "llm" {
			continue
		}
		if accs[g.AgentType] == nil {
			accs[g.AgentType] = &acc{}
		}
		accs[g.AgentType].total++
		if g.Won {
			accs[g.AgentType].wins++
		}
	}

	out := make([]BaselineSolveStat, 0, len(accs))
	for agentType, a := range accs {
		out = append(out, BaselineSolveStat{AgentType: agentType, Total: a.total, Wins: a.wins})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentType < out[j].AgentType })
	return out, nil
}
