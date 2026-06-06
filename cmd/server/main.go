// Command server is the promptevo API entrypoint: load config, open the store,
// run migrations, build the chi router, and serve with graceful shutdown.
// See ARCHITECTURE.md §3 and §12.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"promptevo/internal/experiment"
	"promptevo/internal/llm"
	"promptevo/internal/store"
	"promptevo/internal/wordle"
)

// ─── Config ───────────────────────────────────────────────────────────────────

type config struct {
	Port              int
	DBPath            string
	AnswersPath       string
	GuessesPath       string
	OpenRouterAPIKey  string
	OpenRouterBaseURL string
	LLMTimeoutSeconds int
	MaxConcurrentRuns int
	LogLevel          string
}

func loadConfig() config {
	return config{
		Port:              envInt("PORT", 8080),
		DBPath:            envStr("DB_PATH", "/data/promptevo.db"),
		AnswersPath:       envStr("ANSWERS_PATH", "data/answers.txt"),
		GuessesPath:       envStr("GUESSES_PATH", "data/guesses.txt"),
		OpenRouterAPIKey:  os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterBaseURL: envStr("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		LLMTimeoutSeconds: envInt("LLM_TIMEOUT_SECONDS", 60),
		MaxConcurrentRuns: envInt("MAX_CONCURRENT_RUNS", 2),
		LogLevel:          envStr("LOG_LEVEL", "info"),
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

var selectableModels = []string{
	"anthropic/claude-3.5-sonnet",
	"anthropic/claude-3-haiku",
	"openai/gpt-4o",
	"openai/gpt-4o-mini",
	"google/gemini-2.0-flash",
	"google/gemini-1.5-pro",
	"meta-llama/llama-3.3-70b-instruct",
	"mistralai/mistral-large",
}

var modelSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(selectableModels))
	for _, s := range selectableModels {
		m[s] = struct{}{}
	}
	return m
}()

func isValidModel(m string) bool {
	_, ok := modelSet[m]
	return ok
}

// ─── Server ───────────────────────────────────────────────────────────────────

type server struct {
	cfg   config
	store store.Store
	hub   *experiment.Hub
	orch  *experiment.Orchestrator
	lists *wordle.WordLists
}

func newServer(cfg config, st store.Store, lists *wordle.WordLists, hub *experiment.Hub, orch *experiment.Orchestrator) *server {
	return &server{cfg: cfg, store: st, hub: hub, orch: orch, lists: lists}
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
		r.Get("/models", s.handleListModels)
		r.Get("/runs", s.handleListRuns)
		r.Post("/runs", s.handleCreateRun)
		r.Get("/runs/{id}", s.handleGetRun)
		r.Delete("/runs/{id}", s.handleDeleteRun)
		r.Get("/runs/{id}/generations", s.handleListGenerations)
		r.Get("/runs/{id}/games", s.handleListGames)
		r.Get("/runs/{id}/stream", s.handleStream)
		r.Get("/games/{id}/guesses", s.handleListGuesses)
	})

	return r
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleListModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{"models": selectableModels})
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
	PlayerModel      string  `json:"playerModel"`
	ReflectorModel   string  `json:"reflectorModel"`
	Temperature      *float64 `json:"temperature"`
	Seed             *int64  `json:"seed"`
	Generations      int     `json:"generations"`
	GamesPerGen      int     `json:"gamesPerGen"`
	WordSampleSize   int     `json:"wordSampleSize"`
	IncludeBaselines bool    `json:"includeBaselines"`
}

func (s *server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if s.cfg.OpenRouterAPIKey == "" {
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
	if !isValidModel(req.PlayerModel) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown model: %s", req.PlayerModel))
		return
	}
	if req.ReflectorModel == "" {
		writeError(w, http.StatusBadRequest, "reflectorModel is required")
		return
	}
	if !isValidModel(req.ReflectorModel) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown model: %s", req.ReflectorModel))
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

	// Launch experiment goroutine (StartRun is non-blocking).
	s.orch.StartRun(id)

	writeJSON(w, http.StatusCreated, created)
}

// runDetailResponse is the GET /api/runs/{id} response shape.
type runDetailResponse struct {
	*store.Run
	GenerationsData []*store.Generation `json:"generationsData"`
	Convergence     string              `json:"convergence"`
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
	if run.Status == "completed" || run.Status == "failed" {
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
	playerClient := llm.NewOpenRouterClient(cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, llmTimeout)
	reflectorClient := llm.NewOpenRouterClient(cfg.OpenRouterAPIKey, cfg.OpenRouterBaseURL, llmTimeout)

	hub := experiment.NewHub()
	orch := &experiment.Orchestrator{
		Store:           st,
		PlayerClient:    playerClient,
		ReflectorClient: reflectorClient,
		Lists:           lists,
		Hub:             hub,
	}

	srv := newServer(cfg, st, lists, hub, orch)
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
