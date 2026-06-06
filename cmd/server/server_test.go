// Package main — handler tests.
//
// # Current status
//
// API handlers are stubbed (ARCHITECTURE.md §4 is marked TODO for the Backend
// Developer). Only /healthz is currently wired, so that is the only route
// tested against a real response. The rest of this file:
//   - defines newTestRouter (update its signature to match the final
//     newRouter once the Backend wires handlers and store injection),
//   - asserts the correct status codes and JSON wire format for every endpoint,
//   - uses store.MockStore so no real DB is needed.
//
// Run with: go test ./cmd/server/...
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"promptevo/internal/store"
)

// tCtx returns a context tied to the test (Go 1.22 compatibility shim).
func tCtx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

// newTestRouter builds a router wired with the provided MockStore.
//
// TODO(backend): once newRouter accepts dependency injection, update this to
// call newRouter(ms, hub, lists) instead of newRouter().
// For now we call the no-arg stub so the file compiles immediately.
func newTestRouter(_ *store.MockStore) http.Handler {
	return newRouter()
}

// --- /healthz ---

func TestHealthz_OK(t *testing.T) {
	r := newTestRouter(store.NewMockStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status = %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("GET /healthz: Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("GET /healthz: failed to decode JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("GET /healthz: status field = %q, want ok", body["status"])
	}
}

func TestHealthz_WrongMethod(t *testing.T) {
	r := newTestRouter(store.NewMockStore())

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/healthz", nil)
		r.ServeHTTP(w, req)
		// chi returns 405 for method mismatches on registered routes.
		if w.Code == http.StatusOK {
			t.Errorf("%s /healthz: expected non-200, got 200", method)
		}
	}
}

// --- /api/models ---
// TODO(backend): uncomment once GET /api/models is wired in newRouter.
//
//nolint:unused
func testModels(t *testing.T, r http.Handler) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/models: status = %d, want 200", w.Code)
	}

	var body struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("GET /api/models: failed to decode JSON: %v", err)
	}
	if len(body.Models) == 0 {
		t.Error("GET /api/models: models array is empty, want at least one entry")
	}
	// All entries must be non-empty strings.
	for i, m := range body.Models {
		if m == "" {
			t.Errorf("GET /api/models: models[%d] is empty", i)
		}
	}
}

// --- /api/runs ---
// TODO(backend): uncomment and call these once handlers are wired.

//nolint:unused
func testListRunsEmpty(t *testing.T, r http.Handler) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/runs: status = %d, want 200", w.Code)
	}

	var body struct {
		Runs []json.RawMessage `json:"runs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("GET /api/runs: failed to decode JSON: %v", err)
	}
	// Empty list must be [] not null (ARCHITECTURE.md §4.2)
	if body.Runs == nil {
		t.Error("GET /api/runs: runs field is null, want []")
	}
	if len(body.Runs) != 0 {
		t.Errorf("GET /api/runs: runs len = %d, want 0", len(body.Runs))
	}
}

//nolint:unused
func testCreateRun_MissingField(t *testing.T, r http.Handler) {
	t.Helper()
	// Missing required fields playerModel and reflectorModel.
	payload := `{"generations":5,"gamesPerGen":10,"wordSampleSize":20}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/runs (missing field): status = %d, want 400", w.Code)
	}

	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(w.Body).Decode(&errBody)
	if errBody.Error == "" {
		t.Error("POST /api/runs (missing field): error field is empty")
	}
}

//nolint:unused
func testCreateRun_Valid(t *testing.T, r http.Handler) {
	t.Helper()
	payload := `{
		"playerModel":    "openai/gpt-4o-mini",
		"reflectorModel": "anthropic/claude-3.5-sonnet",
		"temperature":    0.7,
		"seed":           42,
		"generations":    2,
		"gamesPerGen":    5,
		"wordSampleSize": 10
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST /api/runs: status = %d, want 201; body: %s", w.Code, w.Body.String())
	}

	var run struct {
		ID             int64   `json:"id"`
		PlayerModel    string  `json:"playerModel"`
		ReflectorModel string  `json:"reflectorModel"`
		Temperature    float64 `json:"temperature"`
		Status         string  `json:"status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&run); err != nil {
		t.Fatalf("POST /api/runs: failed to decode JSON: %v", err)
	}
	if run.ID == 0 {
		t.Error("POST /api/runs: id is 0, want non-zero")
	}
	if run.PlayerModel != "openai/gpt-4o-mini" {
		t.Errorf("POST /api/runs: playerModel = %q, want openai/gpt-4o-mini", run.PlayerModel)
	}
	if run.Status != "running" && run.Status != "pending" {
		t.Errorf("POST /api/runs: status = %q, want running or pending", run.Status)
	}
}

//nolint:unused
func testGetRun_NotFound(t *testing.T, r http.Handler) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/runs/9999", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/runs/9999: status = %d, want 404", w.Code)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(w.Body).Decode(&errBody)
	if errBody.Error == "" {
		t.Error("GET /api/runs/9999: error field is empty")
	}
}

//nolint:unused
func testDeleteRun_NotFound(t *testing.T, r http.Handler) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/runs/9999", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /api/runs/9999: status = %d, want 404", w.Code)
	}
}

//nolint:unused
func testListGuesses_EmptyGame(t *testing.T, r http.Handler) {
	t.Helper()
	w := httptest.NewRecorder()
	// game ID 9999 doesn't exist in MockStore (empty)
	req := httptest.NewRequest(http.MethodGet, "/api/games/9999/guesses", nil)
	r.ServeHTTP(w, req)

	// Either 404 (game not found) or 200 with empty guesses list is acceptable.
	switch w.Code {
	case http.StatusOK:
		var body struct {
			Guesses []json.RawMessage `json:"guesses"`
		}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("GET /api/games/9999/guesses: decode error: %v", err)
		}
		if body.Guesses == nil {
			t.Error("GET /api/games/9999/guesses: guesses is null, want []")
		}
	case http.StatusNotFound:
		// acceptable
	default:
		t.Errorf("GET /api/games/9999/guesses: status = %d, want 200 or 404", w.Code)
	}
}

// --- MockStore round-trip tests ---
// These run immediately (no handler dependency).

func TestMockStore_RunCRUD(t *testing.T) {
	ms := store.NewMockStore()
	ctx := tCtx(t)

	run := &store.Run{
		PlayerModel:    "openai/gpt-4o-mini",
		ReflectorModel: "anthropic/claude-3.5-sonnet",
		Temperature:    0.7,
		Seed:           42,
		Generations:    5,
		GamesPerGen:    20,
		WordSampleSize: 50,
		Status:         "pending",
	}

	id, err := ms.CreateRun(ctx, run)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if id == 0 {
		t.Error("CreateRun: returned id 0")
	}

	got, err := ms.GetRun(ctx, id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.PlayerModel != run.PlayerModel {
		t.Errorf("GetRun: PlayerModel = %q, want %q", got.PlayerModel, run.PlayerModel)
	}

	if err := ms.UpdateRunStatus(ctx, id, "running"); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}
	got2, _ := ms.GetRun(ctx, id)
	if got2.Status != "running" {
		t.Errorf("UpdateRunStatus: Status = %q, want running", got2.Status)
	}
}

func TestMockStore_ListRunsNewestFirst(t *testing.T) {
	ms := store.NewMockStore()
	ctx := tCtx(t)

	for i := 0; i < 3; i++ {
		_, _ = ms.CreateRun(ctx, &store.Run{Status: "pending"})
	}
	runs, err := ms.ListRuns(ctx)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("ListRuns: len = %d, want 3", len(runs))
	}
	// Newest first: IDs should be descending.
	for i := 0; i+1 < len(runs); i++ {
		if runs[i].ID <= runs[i+1].ID {
			t.Errorf("ListRuns: runs[%d].ID=%d not > runs[%d].ID=%d (want newest first)",
				i, runs[i].ID, i+1, runs[i+1].ID)
		}
	}
}

func TestMockStore_ListRunsEmpty(t *testing.T) {
	ms := store.NewMockStore()
	runs, err := ms.ListRuns(tCtx(t))
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if runs == nil {
		t.Error("ListRuns: returned nil, want empty slice")
	}
	if len(runs) != 0 {
		t.Errorf("ListRuns: len = %d, want 0", len(runs))
	}
}

func TestMockStore_GetRun_NotFound(t *testing.T) {
	ms := store.NewMockStore()
	_, err := ms.GetRun(tCtx(t), 9999)
	if err != store.ErrNotFound {
		t.Errorf("GetRun(9999): error = %v, want ErrNotFound", err)
	}
}

func TestMockStore_DeleteRun_Cascade(t *testing.T) {
	ms := store.NewMockStore()
	ctx := tCtx(t)

	runID, _ := ms.CreateRun(ctx, &store.Run{Status: "pending"})

	gameID, _ := ms.CreateGame(ctx, &store.Game{RunID: runID, Answer: "crane", AgentType: "llm"})

	_, _ = ms.CreateGuess(ctx, &store.Guess{
		GameID:    gameID,
		TurnIndex: 0,
		Guess:     "slate",
		Feedback:  "XXGXG",
	})

	_, _ = ms.CreateGeneration(ctx, &store.Generation{
		RunID:    runID,
		GenIndex: 0,
		PromptText: "You are an expert Wordle player.",
		PromptLen:  30,
	})

	// Delete cascades.
	if err := ms.DeleteRun(ctx, runID); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}

	// Run itself must be gone.
	if _, err := ms.GetRun(ctx, runID); err != store.ErrNotFound {
		t.Errorf("GetRun after DeleteRun: error = %v, want ErrNotFound", err)
	}

	// Games must be gone.
	games, _ := ms.ListGames(ctx, runID, nil)
	if len(games) != 0 {
		t.Errorf("ListGames after DeleteRun: len = %d, want 0", len(games))
	}

	// Guesses for the deleted game must be gone.
	guesses, _ := ms.ListGuesses(ctx, gameID)
	if len(guesses) != 0 {
		t.Errorf("ListGuesses after DeleteRun: len = %d, want 0", len(guesses))
	}

	// Generations must be gone.
	gens, _ := ms.ListGenerations(ctx, runID)
	if len(gens) != 0 {
		t.Errorf("ListGenerations after DeleteRun: len = %d, want 0", len(gens))
	}
}

func TestMockStore_DeleteRun_NotFound(t *testing.T) {
	ms := store.NewMockStore()
	err := ms.DeleteRun(tCtx(t), 9999)
	if err != store.ErrNotFound {
		t.Errorf("DeleteRun(9999): error = %v, want ErrNotFound", err)
	}
}

func TestMockStore_ListGames_GenFilter(t *testing.T) {
	ms := store.NewMockStore()
	ctx := tCtx(t)
	runID, _ := ms.CreateRun(ctx, &store.Run{Status: "pending"})

	_, _ = ms.CreateGame(ctx, &store.Game{RunID: runID, GenIndex: 0, Answer: "crane", AgentType: "llm"})
	_, _ = ms.CreateGame(ctx, &store.Game{RunID: runID, GenIndex: 0, Answer: "slate", AgentType: "llm"})
	_, _ = ms.CreateGame(ctx, &store.Game{RunID: runID, GenIndex: 1, Answer: "adore", AgentType: "llm"})

	gen0 := 0
	games0, _ := ms.ListGames(ctx, runID, &gen0)
	if len(games0) != 2 {
		t.Errorf("ListGames(gen=0): len = %d, want 2", len(games0))
	}

	gen1 := 1
	games1, _ := ms.ListGames(ctx, runID, &gen1)
	if len(games1) != 1 {
		t.Errorf("ListGames(gen=1): len = %d, want 1", len(games1))
	}

	all, _ := ms.ListGames(ctx, runID, nil)
	if len(all) != 3 {
		t.Errorf("ListGames(nil): len = %d, want 3", len(all))
	}
}

func TestMockStore_ListGuesses_OrderedByTurnIndex(t *testing.T) {
	ms := store.NewMockStore()
	ctx := tCtx(t)
	runID, _ := ms.CreateRun(ctx, &store.Run{Status: "pending"})
	gameID, _ := ms.CreateGame(ctx, &store.Game{RunID: runID, Answer: "crane", AgentType: "llm"})

	// Insert out-of-order.
	_, _ = ms.CreateGuess(ctx, &store.Guess{GameID: gameID, TurnIndex: 2, Guess: "crank", Feedback: "GGGGX"})
	_, _ = ms.CreateGuess(ctx, &store.Guess{GameID: gameID, TurnIndex: 0, Guess: "slate", Feedback: "XXGXG"})
	_, _ = ms.CreateGuess(ctx, &store.Guess{GameID: gameID, TurnIndex: 1, Guess: "grace", Feedback: "XGGXG"})

	guesses, _ := ms.ListGuesses(ctx, gameID)
	if len(guesses) != 3 {
		t.Fatalf("ListGuesses: len = %d, want 3", len(guesses))
	}
	for i, g := range guesses {
		if g.TurnIndex != i {
			t.Errorf("guesses[%d].TurnIndex = %d, want %d", i, g.TurnIndex, i)
		}
	}
}

func TestMockStore_ErrorInjection(t *testing.T) {
	ms := store.NewMockStore()
	ctx := tCtx(t)

	// Inject error on GetRun.
	ms.ErrGetRun = store.ErrNotFound
	_, err := ms.GetRun(ctx, 1)
	if err != store.ErrNotFound {
		t.Errorf("GetRun with injected error: got %v, want ErrNotFound", err)
	}
}
