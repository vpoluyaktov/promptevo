// Command server is the promptevo API entrypoint: load config, open the store,
// run migrations, build the chi router, and serve with graceful shutdown.
// See ARCHITECTURE.md §3 and §12.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"promptevo/internal/experiment"
	"promptevo/internal/llm"
	"promptevo/internal/reflector"
	"promptevo/internal/store"
	"promptevo/internal/wordle"
)

// ─── Config ───────────────────────────────────────────────────────────────────

type config struct {
	Port              int
	DBPath            string
	AnswersPath       string
	GuessesPath       string
	LLMProvider       string // "openrouter" | "anthropic" | "openai"  default: "openrouter"
	OpenRouterAPIKey  string
	OpenRouterBaseURL string
	AnthropicAPIKey   string
	OpenAIAPIKey      string
	LLMTimeoutSeconds int
	MaxConcurrentRuns int
	LogLevel          string
	AuthUsername      string
	AuthPassword      string
}

func loadConfig() config {
	return config{
		Port:              envInt("PORT", 8080),
		DBPath:            envStr("DB_PATH", "/data/promptevo.db"),
		AnswersPath:       envStr("ANSWERS_PATH", "data/answers.txt"),
		GuessesPath:       envStr("GUESSES_PATH", "data/guesses.txt"),
		LLMProvider:       envStr("LLM_PROVIDER", "openrouter"),
		OpenRouterAPIKey:  os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterBaseURL: envStr("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		AnthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		LLMTimeoutSeconds: envInt("LLM_TIMEOUT_SECONDS", 60),
		MaxConcurrentRuns: envInt("MAX_CONCURRENT_RUNS", 2),
		LogLevel:          envStr("LOG_LEVEL", "info"),
		AuthUsername:      os.Getenv("AUTH_USERNAME"),
		AuthPassword:      os.Getenv("AUTH_PASSWORD"),
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

// ─── Selectable models ────────────────────────────────────────────────────────

// fallbackModels returns a hardcoded model list used when the provider API is
// unreachable (no key set, network error, etc.).
func fallbackModels(provider string) []string {
	switch provider {
	case "anthropic":
		return []string{
			"claude-opus-4-8",
			"claude-sonnet-4-6",
			"claude-haiku-4-5-20251001",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
		}
	case "openai":
		return []string{
			"gpt-4o",
			"gpt-4o-mini",
			"o3-mini",
			"o1",
		}
	default: // openrouter
		return []string{
			"openai/gpt-4o-mini",
			"openai/gpt-4o",
			"google/gemini-2.0-flash-001",
			"meta-llama/llama-3.3-70b-instruct",
			"anthropic/claude-3-5-sonnet-20241022",
		}
	}
}

// ─── Server ───────────────────────────────────────────────────────────────────

type server struct {
	cfg         config
	store       store.Store
	hub         *experiment.Hub
	orch        *experiment.Orchestrator
	lists       *wordle.WordLists
	authToken   string // empty when auth is disabled
	mu          sync.Mutex
	cancelFuncs map[int64]context.CancelFunc
}

func newServer(cfg config, st store.Store, lists *wordle.WordLists, hub *experiment.Hub, orch *experiment.Orchestrator, authToken string) *server {
	return &server{
		cfg:         cfg,
		store:       st,
		hub:         hub,
		orch:        orch,
		lists:       lists,
		authToken:   authToken,
		cancelFuncs: make(map[int64]context.CancelFunc),
	}
}

// activeRunCount counts runs with status "running" or "pending" in the store.
func (s *server) activeRunCount(ctx context.Context) int {
	runs, err := s.store.ListRuns(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range runs {
		if r.Status == "running" || r.Status == "pending" {
			n++
		}
	}
	return n
}

// ─── Router ───────────────────────────────────────────────────────────────────

// newRouter returns a chi router wired with s's handlers. It is also exposed
// as a package-level function (called with a nil/zero server) so that tests
// which only exercise dependency-free routes (e.g. /healthz) can call
// newRouter() without building a full server. Handlers that access s fields
// are safe as long as those handlers are not exercised in the minimal case.
func newRouter() http.Handler {
	return new(server).buildRoutes()
}

func (s *server) buildRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/healthz", s.handleHealthz)

	r.Route("/api", func(r chi.Router) {
		// Public: login endpoint (no auth required)
		r.Post("/auth/login", s.handleLogin)

		// Protected: all other API routes require a valid Bearer token
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/models", s.handleListModels)
			r.Get("/prompts", s.handleGetDefaultPrompts)
			r.Get("/runs", s.handleListRuns)
			r.Post("/runs", s.handleCreateRun)
			r.Get("/runs/{id}", s.handleGetRun)
			r.Post("/runs/{id}/stop", s.handleStopRun)
			r.Delete("/runs/{id}", s.handleDeleteRun)
			r.Get("/runs/{id}/generations", s.handleListGenerations)
			r.Get("/runs/{id}/games", s.handleListGames)
			r.Get("/runs/{id}/analytics", s.handleGetAnalytics)
			r.Get("/runs/{id}/stream", s.handleStream)
			r.Get("/games/{id}/guesses", s.handleListGuesses)
		})
	})

	return r
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLogin handles POST /api/auth/login.
// When auth is disabled it returns a dummy token so the frontend works uniformly.
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if s.authToken == "" {
		// Auth disabled — return a dummy token so the frontend works uniformly.
		writeJSON(w, http.StatusOK, map[string]string{"token": "disabled"})
		return
	}

	userMatch := subtle.ConstantTimeCompare([]byte(body.Username), []byte(s.cfg.AuthUsername))
	passMatch := subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.cfg.AuthPassword))
	if userMatch+passMatch != 2 {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": s.authToken})
}

// requireAuth is middleware that enforces Bearer token authentication on
// protected routes. When auth is disabled (authToken == "") all requests pass.
func (s *server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authToken == "" {
			// Auth disabled — pass through.
			next.ServeHTTP(w, r)
			return
		}
		// EventSource (SSE) cannot send custom headers, so fall back to
		// ?token= query param for the stream endpoint.
		var token string
		const prefix = "Bearer "
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, prefix) {
			token = strings.TrimPrefix(h, prefix)
		} else {
			token = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) handleListModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	models, err := llm.ListModels(ctx, s.cfg.LLMProvider, s.activeAPIKey(), s.cfg.OpenRouterBaseURL, 10*time.Second)
	if err != nil || len(models) == 0 {
		models = fallbackModels(s.cfg.LLMProvider)
	}
	sort.Strings(models)
	writeJSON(w, http.StatusOK, map[string][]string{"models": models})
}

// handleGetDefaultPrompts handles GET /api/prompts?maxGuesses=N.
// Returns the generation-0 player strategy prompt and the reflector system prompt.
func (s *server) handleGetDefaultPrompts(w http.ResponseWriter, r *http.Request) {
	maxGuesses := 3
	if v := r.URL.Query().Get("maxGuesses"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 2 && n <= 6 {
			maxGuesses = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"playerPrompt":    experiment.DefaultStrategyPrompt(maxGuesses),
		"reflectorPrompt": reflector.SystemPrompt(),
	})
}

// activeAPIKey returns the API key for the configured LLM provider.
func (s *server) activeAPIKey() string {
	return apiKeyForProvider(s.cfg)
}

func apiKeyForProvider(cfg config) string {
	switch cfg.LLMProvider {
	case "anthropic":
		return cfg.AnthropicAPIKey
	case "openai":
		return cfg.OpenAIAPIKey
	default:
		return cfg.OpenRouterAPIKey
	}
}

func (s *server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string][]*store.Run{"runs": runs})
}

// createRunRequest is the POST /api/runs body.
type createRunRequest struct {
	PlayerModel      string   `json:"playerModel"`
	ReflectorModel   string   `json:"reflectorModel"`
	Temperature      *float64 `json:"temperature"`
	Seed             *int64   `json:"seed"`
	Generations      int      `json:"generations"`
	GamesPerGen      int      `json:"gamesPerGen"`
	WordSampleSize   int      `json:"wordSampleSize"`
	MaxGuesses       int      `json:"maxGuesses"`
	IncludeBaselines bool     `json:"includeBaselines"`
}

func (s *server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if s.activeAPIKey() == "" {
		writeError(w, http.StatusServiceUnavailable, "LLM gateway not configured")
		return
	}

	var req createRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Defaults.
	temperature := 0.7
	if req.Temperature != nil {
		temperature = *req.Temperature
	}
	seed := int64(42)
	if req.Seed != nil {
		seed = *req.Seed
	}

	// Validation.
	if req.PlayerModel == "" {
		writeError(w, http.StatusBadRequest, "playerModel is required")
		return
	}
	if req.ReflectorModel == "" {
		writeError(w, http.StatusBadRequest, "reflectorModel is required")
		return
	}
	if temperature < 0 || temperature > 2.0 {
		writeError(w, http.StatusBadRequest, "temperature must be between 0.0 and 2.0")
		return
	}
	if req.Generations < 1 || req.Generations > 50 {
		writeError(w, http.StatusBadRequest, "generations must be between 1 and 50")
		return
	}
	if req.GamesPerGen < 1 || req.GamesPerGen > 500 {
		writeError(w, http.StatusBadRequest, "gamesPerGen must be between 1 and 500")
		return
	}
	maxSample := len(s.lists.Answers)
	if req.WordSampleSize < 1 || req.WordSampleSize > maxSample {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("wordSampleSize must be between 1 and %d", maxSample))
		return
	}
	maxGuesses := req.MaxGuesses
	if maxGuesses == 0 {
		maxGuesses = 4 // default: hard mode
	}
	if maxGuesses < 2 || maxGuesses > 6 {
		writeError(w, http.StatusBadRequest, "maxGuesses must be between 2 and 6")
		return
	}

	// Concurrency check.
	if s.activeRunCount(r.Context()) >= s.cfg.MaxConcurrentRuns {
		writeError(w, http.StatusServiceUnavailable, "too many concurrent runs")
		return
	}

	cfgJSON, err := json.Marshal(experiment.RunConfig{IncludeBaselines: req.IncludeBaselines})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	run := &store.Run{
		PlayerModel:    req.PlayerModel,
		ReflectorModel: req.ReflectorModel,
		Temperature:    temperature,
		Seed:           seed,
		Generations:    req.Generations,
		GamesPerGen:    req.GamesPerGen,
		WordSampleSize: req.WordSampleSize,
		MaxGuesses:     maxGuesses,
		Status:         "pending",
		ConfigJSON:     string(cfgJSON),
	}

	id, err := s.store.CreateRun(r.Context(), run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create run")
		return
	}

	// Re-fetch to get the DB-generated created_at.
	created, err := s.store.GetRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch run")
		return
	}

	// Launch experiment goroutine with a cancellable context so the run can
	// be stopped via POST /api/runs/{id}/stop.
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancelFuncs[id] = cancel
	s.mu.Unlock()

	s.orch.StartRun(runCtx, id, func() {
		s.mu.Lock()
		delete(s.cancelFuncs, id)
		s.mu.Unlock()
		cancel() // safe to call twice; frees context resources
	})

	writeJSON(w, http.StatusCreated, created)
}

// runDetailResponse is the GET /api/runs/{id} response shape.
type runDetailResponse struct {
	*store.Run
	GenerationsData []*store.Generation `json:"generationsData"`
	Convergence     string              `json:"convergence"`
}

// ─── Analytics response types ─────────────────────────────────────────────────

type AnalyticsResponse struct {
	RunID               int64                  `json:"runId"`
	MaxGuesses          int                    `json:"maxGuesses"`
	Meta                AnalyticsMeta          `json:"meta"`
	SolveRateCI         []SolveRateCIPoint     `json:"solveRateCI"`
	WinDistribution     []WinDistPoint         `json:"winDistribution"`
	TurnInfoGain        []TurnInfoGainPoint    `json:"turnInfoGain"`
	OpeningWords        []OpeningWordsPoint    `json:"openingWords"`
	RemainingCandidates []RemainingCandPoint   `json:"remainingCandidates"`
	TokenEfficiency     []TokenEfficiencyPoint `json:"tokenEfficiency"`
	ReasoningVerbosity  []ReasoningPoint       `json:"reasoningVerbosity"`
	WordDifficulty      []WordDifficultyPoint  `json:"wordDifficulty"`
	BaselineStats       []BaselineStatPoint    `json:"baselineStats"`
}

type BaselineStatPoint struct {
	AgentType string  `json:"agentType"`
	Total     int     `json:"total"`
	Wins      int     `json:"wins"`
	SolveRate float64 `json:"solveRate"`
}

type AnalyticsMeta struct {
	TotalLlmGames int `json:"totalLlmGames"`
	Generations   int `json:"generations"`
}

type SolveRateCIPoint struct {
	GenIndex  int     `json:"genIndex"`
	N         int     `json:"n"`
	Wins      int     `json:"wins"`
	SolveRate float64 `json:"solveRate"`
	CILower   float64 `json:"ciLower"`
	CIUpper   float64 `json:"ciUpper"`
}

type WinDistPoint struct {
	GenIndex  int         `json:"genIndex"`
	Total     int         `json:"total"`
	WonByTurn map[int]int `json:"wonByTurn"` // marshals keys as strings
	Lost      int         `json:"lost"`
}

type TurnInfoGainPoint struct {
	GenIndex     int     `json:"genIndex"`
	TurnIndex    int     `json:"turnIndex"`
	MeanInfoGain float64 `json:"meanInfoGain"`
	N            int     `json:"n"`
}

type OpeningWordsPoint struct {
	GenIndex int              `json:"genIndex"`
	Words    []OpeningWordRow `json:"words"`
}

type OpeningWordRow struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type RemainingCandPoint struct {
	GameID              int64  `json:"gameId"`
	GenIndex            int    `json:"genIndex"`
	Answer              string `json:"answer"`
	RemainingCandidates int    `json:"remainingCandidates"`
	NumGuesses          int    `json:"numGuesses"`
}

type TokenEfficiencyPoint struct {
	GenIndex        int  `json:"genIndex"`
	PlayerTokens    int  `json:"playerTokens"`
	ReflectorTokens int  `json:"reflectorTokens"`
	TokensUsed      int  `json:"tokensUsed"`
	Split           bool `json:"split"`
}

type ReasoningPoint struct {
	GameID         int64 `json:"gameId"`
	GenIndex       int   `json:"genIndex"`
	Won            bool  `json:"won"`
	ReasoningChars int   `json:"reasoningChars"`
	NumGuesses     int   `json:"numGuesses"`
}

type WordDifficultyPoint struct {
	Answer  string  `json:"answer"`
	Games   int     `json:"games"`
	Wins    int     `json:"wins"`
	WinRate float64 `json:"winRate"`
}

// wilson computes the 95% Wilson score confidence interval for wins out of n trials.
// Returns (0, 0) when n == 0 (caller should omit the row).
// Both bounds are clamped to [0, 1].
func wilson(wins, n int) (lower, upper float64) {
	if n == 0 {
		return 0, 0
	}
	p := float64(wins) / float64(n)
	z := 1.96
	z2 := z * z
	fn := float64(n)
	denom := 1 + z2/fn
	center := (p + z2/(2*fn)) / denom
	margin := (z / denom) * math.Sqrt(p*(1-p)/fn+z2/(4*fn*fn))
	lower = clamp01(center - margin)
	upper = clamp01(center + margin)
	return lower, upper
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// round3 rounds a float64 to 3 decimal places.
func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// round2 rounds a float64 to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func (s *server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	run, err := s.store.GetRun(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	gens, err := s.store.ListGenerations(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list generations")
		return
	}

	var rates []float64
	for _, g := range gens {
		if g.SolveRate != nil {
			rates = append(rates, *g.SolveRate)
		}
	}

	writeJSON(w, http.StatusOK, runDetailResponse{
		Run:             run,
		GenerationsData: gens,
		Convergence:     experiment.Convergence(rates),
	})
}

func (s *server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if _, err := s.store.GetRun(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	if err := s.store.DeleteRun(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete run")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// handleStopRun handles POST /api/runs/{id}/stop.
// It cancels the experiment goroutine for a running run and returns 200 {"status":"stopped"}.
func (s *server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	run, err := s.store.GetRun(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	if run.Status != "running" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("run is not running (status: %s)", run.Status))
		return
	}

	s.mu.Lock()
	cancel, active := s.cancelFuncs[id]
	s.mu.Unlock()
	if !active {
		// Status is "running" in the DB but the goroutine is gone (e.g. server
		// restarted). Treat as not stoppable.
		writeError(w, http.StatusBadRequest, "run not active")
		return
	}
	cancel()

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *server) handleListGenerations(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if _, err := s.store.GetRun(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	gens, err := s.store.ListGenerations(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list generations")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]*store.Generation{"generations": gens})
}

func (s *server) handleListGames(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var genFilter *int
	if genStr := r.URL.Query().Get("gen"); genStr != "" {
		n, err := strconv.Atoi(genStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid gen parameter")
			return
		}
		genFilter = &n
	}

	if _, err := s.store.GetRun(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	games, err := s.store.ListGames(r.Context(), id, genFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list games")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]*store.Game{"games": games})
}

func (s *server) handleListGuesses(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	guesses, err := s.store.ListGuesses(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list guesses")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]*store.Guess{"guesses": guesses})
}

// ─── Analytics handler ────────────────────────────────────────────────────────

func (s *server) handleGetAnalytics(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	run, err := s.store.GetRun(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}
	resp, err := s.computeAnalytics(r.Context(), run)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute analytics")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) computeAnalytics(ctx context.Context, run *store.Run) (*AnalyticsResponse, error) {
	runID := run.ID
	maxGuesses := run.MaxGuesses

	// 1. Generations (for token efficiency)
	gens, err := s.store.ListGenerations(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list generations: %w", err)
	}

	// 2. GameOutcomeCounts (feeds CI + win distribution)
	outcomeCounts, err := s.store.GameOutcomeCounts(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("game outcome counts: %w", err)
	}

	// 3. TurnInfoGainStats
	turnStats, err := s.store.TurnInfoGainStats(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("turn info gain stats: %w", err)
	}

	// 4. OpeningWordCounts
	openingCounts, err := s.store.OpeningWordCounts(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("opening word counts: %w", err)
	}

	// 5. ReasoningVerbosityStats
	reasoningStats, err := s.store.ReasoningVerbosityStats(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("reasoning verbosity stats: %w", err)
	}

	// 6. WordDifficultyStats
	wordStats, err := s.store.WordDifficultyStats(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("word difficulty stats: %w", err)
	}

	// 7. Remaining candidates replay: need all LLM games + guesses per lost game
	allGames, err := s.store.ListGames(ctx, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("list games for replay: %w", err)
	}

	// 8. Baseline agent outcomes
	baselineRows, err := s.store.BaselineOutcomeCounts(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("baseline outcome counts: %w", err)
	}

	// ── Build SolveRateCI + WinDistribution from outcomeCounts ───────────────

	type genAcc struct {
		n    int
		wins int
		// wonByTurn: initialized with all turns present
		wonByTurn map[int]int
		lost      int
	}
	genAccs := make(map[int]*genAcc)

	// Ensure every gen that appears has all turns initialized.
	ensureGen := func(gi int) *genAcc {
		if genAccs[gi] == nil {
			wbt := make(map[int]int, maxGuesses)
			for t := 1; t <= maxGuesses; t++ {
				wbt[t] = 0
			}
			genAccs[gi] = &genAcc{wonByTurn: wbt}
		}
		return genAccs[gi]
	}

	for _, oc := range outcomeCounts {
		ga := ensureGen(oc.GenIndex)
		ga.n += oc.Count
		if oc.Won {
			ga.wins += oc.Count
			// Clamp numGuesses to [1, maxGuesses] defensively.
			t := oc.NumGuesses
			if t < 1 {
				t = 1
			}
			if t > maxGuesses {
				t = maxGuesses
			}
			ga.wonByTurn[t] += oc.Count
		} else {
			ga.lost += oc.Count
		}
	}

	// Collect gen indices sorted.
	var genIndices []int
	for gi := range genAccs {
		genIndices = append(genIndices, gi)
	}
	sort.Ints(genIndices)

	solveRateCI := make([]SolveRateCIPoint, 0)
	winDist := make([]WinDistPoint, 0)
	totalLlmGames := 0

	for _, gi := range genIndices {
		ga := genAccs[gi]
		totalLlmGames += ga.n
		if ga.n > 0 {
			lo, hi := wilson(ga.wins, ga.n)
			solveRateCI = append(solveRateCI, SolveRateCIPoint{
				GenIndex:  gi,
				N:         ga.n,
				Wins:      ga.wins,
				SolveRate: round3(float64(ga.wins) / float64(ga.n)),
				CILower:   round3(lo),
				CIUpper:   round3(hi),
			})
		}
		winDist = append(winDist, WinDistPoint{
			GenIndex:  gi,
			Total:     ga.n,
			WonByTurn: ga.wonByTurn,
			Lost:      ga.lost,
		})
	}

	// ── Build TurnInfoGain ────────────────────────────────────────────────────

	turnInfoGain := make([]TurnInfoGainPoint, 0, len(turnStats))
	for _, t := range turnStats {
		turnInfoGain = append(turnInfoGain, TurnInfoGainPoint{
			GenIndex:     t.GenIndex,
			TurnIndex:    t.TurnIndex,
			MeanInfoGain: round2(t.MeanInfoGain),
			N:            t.N,
		})
	}

	// ── Build OpeningWords ────────────────────────────────────────────────────

	openingWords := make([]OpeningWordsPoint, 0)
	var curOWGen *OpeningWordsPoint
	for _, ow := range openingCounts {
		if curOWGen == nil || curOWGen.GenIndex != ow.GenIndex {
			openingWords = append(openingWords, OpeningWordsPoint{
				GenIndex: ow.GenIndex,
				Words:    make([]OpeningWordRow, 0),
			})
			curOWGen = &openingWords[len(openingWords)-1]
		}
		curOWGen.Words = append(curOWGen.Words, OpeningWordRow{
			Word:  ow.Guess,
			Count: ow.Count,
		})
	}

	// ── Build TokenEfficiency from generations ────────────────────────────────

	tokenEfficiency := make([]TokenEfficiencyPoint, 0, len(gens))
	for _, g := range gens {
		split := (g.PlayerTokens + g.ReflectorTokens) > 0
		tokenEfficiency = append(tokenEfficiency, TokenEfficiencyPoint{
			GenIndex:        g.GenIndex,
			PlayerTokens:    g.PlayerTokens,
			ReflectorTokens: g.ReflectorTokens,
			TokensUsed:      g.TokensUsed,
			Split:           split,
		})
	}

	// ── Build ReasoningVerbosity ──────────────────────────────────────────────

	reasoningVerbosity := make([]ReasoningPoint, 0, len(reasoningStats))
	for _, rs := range reasoningStats {
		reasoningVerbosity = append(reasoningVerbosity, ReasoningPoint{
			GameID:         rs.GameID,
			GenIndex:       rs.GenIndex,
			Won:            rs.Won,
			ReasoningChars: rs.ReasoningChars,
			NumGuesses:     rs.NumGuesses,
		})
	}

	// ── Build WordDifficulty ──────────────────────────────────────────────────

	wordDifficulty := make([]WordDifficultyPoint, 0, len(wordStats))
	for _, wd := range wordStats {
		var winRate float64
		if wd.Games > 0 {
			winRate = round3(float64(wd.Wins) / float64(wd.Games))
		}
		wordDifficulty = append(wordDifficulty, WordDifficultyPoint{
			Answer:  wd.Answer,
			Games:   wd.Games,
			Wins:    wd.Wins,
			WinRate: winRate,
		})
	}

	// ── Remaining candidates replay (lost LLM games only) ────────────────────

	remainingCandidates := make([]RemainingCandPoint, 0)
	for _, g := range allGames {
		if g.AgentType != "llm" || g.Won {
			continue
		}
		guesses, err := s.store.ListGuesses(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("list guesses for game %d: %w", g.ID, err)
		}
		candidates := make([]string, len(s.lists.Answers))
		copy(candidates, s.lists.Answers)
		for _, gu := range guesses {
			fb := wordle.FromString(gu.Feedback)
			candidates = wordle.FilterCandidates(candidates, gu.Guess, fb)
		}
		remainingCandidates = append(remainingCandidates, RemainingCandPoint{
			GameID:              g.ID,
			GenIndex:            g.GenIndex,
			Answer:              g.Answer,
			RemainingCandidates: len(candidates),
			NumGuesses:          g.NumGuesses,
		})
	}

	// ── Count distinct gen indices across all LLM games ───────────────────────

	genSet := make(map[int]struct{})
	for _, oc := range outcomeCounts {
		genSet[oc.GenIndex] = struct{}{}
	}

	baselineStats := make([]BaselineStatPoint, 0, len(baselineRows))
	for _, b := range baselineRows {
		var sr float64
		if b.Total > 0 {
			sr = round3(float64(b.Wins) / float64(b.Total))
		}
		baselineStats = append(baselineStats, BaselineStatPoint{
			AgentType: b.AgentType,
			Total:     b.Total,
			Wins:      b.Wins,
			SolveRate: sr,
		})
	}

	return &AnalyticsResponse{
		RunID:               runID,
		MaxGuesses:          maxGuesses,
		Meta:                AnalyticsMeta{TotalLlmGames: totalLlmGames, Generations: len(genSet)},
		SolveRateCI:         solveRateCI,
		WinDistribution:     winDist,
		TurnInfoGain:        turnInfoGain,
		OpeningWords:        openingWords,
		RemainingCandidates: remainingCandidates,
		TokenEfficiency:     tokenEfficiency,
		ReasoningVerbosity:  reasoningVerbosity,
		WordDifficulty:      wordDifficulty,
		BaselineStats:       baselineStats,
	}, nil
}

// ─── SSE stream handler ───────────────────────────────────────────────────────

func (s *server) handleStream(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	run, err := s.store.GetRun(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, canFlush := w.(http.Flusher)

	// Already finished — replay terminal run_end and close.
	if run.Status == "completed" || run.Status == "failed" || run.Status == "stopped" {
		gens, _ := s.store.ListGenerations(r.Context(), id)
		var rates []float64
		for _, g := range gens {
			if g.SolveRate != nil {
				rates = append(rates, *g.SolveRate)
			}
		}
		writeSSEEvent(w, experiment.Event{
			Type:        "run_end",
			RunID:       id,
			Status:      run.Status,
			Convergence: experiment.Convergence(rates),
		})
		if canFlush {
			flusher.Flush()
		}
		return
	}

	ch, unsub := s.hub.Subscribe(id)
	defer unsub()

	heartbeat := time.NewTicker(experiment.HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, more := <-ch:
			if !more {
				return
			}
			writeSSEEvent(w, ev)
			if canFlush {
				flusher.Flush()
			}
			if ev.Type == "run_end" {
				return
			}
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, ev experiment.Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseID(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	raw := chi.URLParam(r, param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()
	log.Printf("promptevo starting on port %d", cfg.Port)

	// Open store (fall back to local file if the volume path isn't mounted).
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Printf("WARN: could not open %q (%v); falling back to promptevo.db", cfg.DBPath, err)
		st, err = store.Open("promptevo.db")
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
	}
	defer st.Close()

	if err := st.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	lists, err := wordle.LoadWordLists(cfg.AnswersPath, cfg.GuessesPath)
	if err != nil {
		log.Fatalf("load word lists: %v", err)
	}
	log.Printf("loaded %d answers, %d valid guesses", len(lists.Answers), len(lists.Guesses))

	llmTimeout := time.Duration(cfg.LLMTimeoutSeconds) * time.Second

	activeAPIKey := apiKeyForProvider(cfg)
	if activeAPIKey == "" {
		log.Printf("WARN: no API key set for LLM provider %q — run creation will be unavailable", cfg.LLMProvider)
	}

	playerClient, err := llm.NewClientForProvider(cfg.LLMProvider, activeAPIKey, llmTimeout)
	if err != nil {
		log.Fatalf("create LLM player client: %v", err)
	}
	reflectorClient, err := llm.NewClientForProvider(cfg.LLMProvider, activeAPIKey, llmTimeout)
	if err != nil {
		log.Fatalf("create LLM reflector client: %v", err)
	}

	hub := experiment.NewHub()
	orch := &experiment.Orchestrator{
		Store:           st,
		PlayerClient:    playerClient,
		ReflectorClient: reflectorClient,
		Lists:           lists,
		Hub:             hub,
	}

	// Generate a per-startup server secret and derive the auth token from it.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatal("failed to generate auth secret:", err)
	}
	var authToken string
	if cfg.AuthUsername != "" && cfg.AuthPassword != "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%x", cfg.AuthUsername, cfg.AuthPassword, secret)))
		authToken = hex.EncodeToString(sum[:])
		log.Println("auth: enabled for user", cfg.AuthUsername)
	} else {
		log.Println("auth: disabled (AUTH_USERNAME/AUTH_PASSWORD not set)")
	}

	srv := newServer(cfg, st, lists, hub, orch, authToken)
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv.buildRoutes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // disabled: SSE connections are long-lived
		IdleTimeout:  120 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("listening on :%d", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-sigCh
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("stopped")
}
