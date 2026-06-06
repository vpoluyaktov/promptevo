// Package experiment orchestrates a run: it loops generations x games, drives
// the agent and reflector, persists rows, and publishes SSE events through a Hub.
// See ARCHITECTURE.md §3 and §7.
package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"promptevo/internal/agent"
	"promptevo/internal/baselines"
	"promptevo/internal/llm"
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

// RunConfig is the subset of run parameters stored in config_json.
type RunConfig struct {
	IncludeBaselines bool `json:"includeBaselines"`
}

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

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

// float64Ptr returns a pointer to f.
func float64Ptr(f float64) *float64 { return &f }

// ─── Hub ──────────────────────────────────────────────────────────────────────

// Hub fans out SSE events to subscribers keyed by runID.
type Hub struct {
	mu          sync.Mutex
	subscribers map[int64]map[chan Event]struct{}
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{subscribers: map[int64]map[chan Event]struct{}{}}
}

// Subscribe returns a buffered channel receiving events for runID and an
// unsubscribe function. The caller must call unsubscribe when done.
func (h *Hub) Subscribe(runID int64) (<-chan Event, func()) {
	ch := make(chan Event, 100)
	h.mu.Lock()
	if h.subscribers[runID] == nil {
		h.subscribers[runID] = make(map[chan Event]struct{})
	}
	h.subscribers[runID][ch] = struct{}{}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subscribers[runID], ch)
		if len(h.subscribers[runID]) == 0 {
			delete(h.subscribers, runID)
		}
		close(ch)
	}
	return ch, unsub
}

// Publish delivers ev to all subscribers of the run identified by runID.
// Non-blocking: if a subscriber's channel is full, the event is dropped for
// that subscriber (prevents slow clients from blocking the experiment).
func (h *Hub) Publish(runID int64, ev Event) {
	h.mu.Lock()
	subs := h.subscribers[runID]
	// Copy the map to release the lock before sending.
	chs := make([]chan Event, 0, len(subs))
	for ch := range subs {
		chs = append(chs, ch)
	}
	h.mu.Unlock()

	for _, ch := range chs {
		select {
		case ch <- ev:
		default:
			// Subscriber is slow; drop the event rather than block.
		}
	}
}

// ─── Orchestrator ─────────────────────────────────────────────────────────────

// Orchestrator runs experiments. It holds LLM clients and creates per-run
// Agent and Reflector instances with the model/temperature specified by each run.
type Orchestrator struct {
	Store           store.Store
	PlayerClient    llm.Client
	ReflectorClient llm.Client
	Lists           *wordle.WordLists
	Hub             *Hub
}

// StartRun launches the run goroutine for runID. ctx should be a cancellable
// context — calling its cancel function stops the experiment. onDone is called
// when the goroutine exits (successful, failed, or cancelled).
func (o *Orchestrator) StartRun(ctx context.Context, runID int64, onDone func()) {
	go func() {
		defer onDone()
		// Use a separate background context for cleanup writes so that a
		// cancelled ctx doesn't prevent the final status update.
		cleanupCtx := context.Background()
		err := o.runExperiment(ctx, runID)
		if err == nil {
			return
		}
		if errors.Is(err, context.Canceled) {
			log.Printf("run %d: stopped by request", runID)
			_ = o.Store.UpdateRunStatus(cleanupCtx, runID, "stopped")
			o.Hub.Publish(runID, Event{
				Type:   "run_end",
				RunID:  runID,
				Status: "stopped",
			})
			return
		}
		log.Printf("run %d failed: %v", runID, err)
		_ = o.Store.UpdateRunStatus(cleanupCtx, runID, "failed")
		o.Hub.Publish(runID, Event{
			Type:    "error",
			RunID:   runID,
			Message: fmt.Sprintf("run failed: %v", err),
		})
		o.Hub.Publish(runID, Event{
			Type:        "run_end",
			RunID:       runID,
			Status:      "failed",
			Convergence: "improving",
		})
	}()
}

// runExperiment is the synchronous core of the experiment loop.
func (o *Orchestrator) runExperiment(ctx context.Context, runID int64) error {
	run, err := o.Store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("get run %d: %w", runID, err)
	}

	var cfg RunConfig
	if run.ConfigJSON != "" && run.ConfigJSON != "{}" {
		if err := json.Unmarshal([]byte(run.ConfigJSON), &cfg); err != nil {
			log.Printf("run %d: could not parse config_json: %v", runID, err)
		}
	}

	if err := o.Store.UpdateRunStatus(ctx, runID, "running"); err != nil {
		return fmt.Errorf("mark running: %w", err)
	}

	// Create per-run agent and reflector with the run's model and temperature.
	a := &agent.Agent{
		Client:      o.PlayerClient,
		Lists:       o.Lists,
		Model:       run.PlayerModel,
		Temperature: run.Temperature,
	}
	refl := &reflector.Reflector{
		Client:      o.ReflectorClient,
		Model:       run.ReflectorModel,
		Temperature: run.Temperature,
	}

	// Build the seeded word sample (same sample used every generation).
	sample := sampleWords(o.Lists.Answers, run.Seed, run.WordSampleSize)

	currentPrompt := DefaultStrategyPrompt
	var solveRates []float64

	for genIdx := 0; genIdx < run.Generations; genIdx++ {
		genPromptLen := len([]rune(currentPrompt))
		gen := &store.Generation{
			RunID:      runID,
			GenIndex:   genIdx,
			PromptText: currentPrompt,
			PromptLen:  genPromptLen,
		}
		genID, err := o.Store.CreateGeneration(ctx, gen)
		if err != nil {
			return fmt.Errorf("create generation %d: %w", genIdx, err)
		}
		gen.ID = genID

		// ── Play LLM games ──────────────────────────────────────────────
		type gameResult struct {
			won        bool
			numGuesses int
			infoGain   float64
			violations int
		}
		results := make([]gameResult, 0, run.GamesPerGen)
		totalTokens := 0
		var failedSamples []string

		gameWords := sample
		if len(gameWords) > run.GamesPerGen {
			gameWords = gameWords[:run.GamesPerGen]
		}

		for _, answer := range gameWords {
			gr, err := o.playLLMGame(ctx, a, runID, genIdx, currentPrompt, answer)
			if err != nil {
				// Non-fatal: log and continue.
				log.Printf("run %d gen %d: LLM game error for %q: %v", runID, genIdx, answer, err)
				continue
			}
			results = append(results, gameResult{
				won:        gr.won,
				numGuesses: gr.numGuesses,
				infoGain:   gr.infoGain,
				violations: gr.violations,
			})
			totalTokens += gr.tokensUsed
			if !gr.won && len(failedSamples) < 3 {
				failedSamples = append(failedSamples, gr.transcript)
			}
		}

		// ── Baseline games (gen 0 only when requested) ───────────────────
		if cfg.IncludeBaselines && genIdx == 0 {
			players := []baselines.Player{
				baselines.NewRandomPlayer(run.Seed),
				baselines.NewFrequencyPlayer(),
				baselines.NewEntropyPlayer(),
			}
			for _, player := range players {
				for _, answer := range gameWords {
					if err := o.playBaselineGame(ctx, runID, genIdx, answer, player); err != nil {
						log.Printf("run %d baseline %s %q: %v", runID, player.Name(), answer, err)
					}
				}
			}
		}

		// ── Compute generation metrics ────────────────────────────────────
		var (
			solveRate     float64
			meanGuesses   float64
			meanInfoGain  float64
			violationRate float64
		)
		if n := len(results); n > 0 {
			var won, guessSum, infoSum, violSum float64
			for _, res := range results {
				if res.won {
					won++
				}
				guessSum += float64(res.numGuesses)
				infoSum += res.infoGain
				violSum += float64(res.violations)
			}
			solveRate = won / float64(n)
			meanGuesses = guessSum / float64(n)
			meanInfoGain = infoSum / float64(n)
			violationRate = violSum / float64(n)
		}
		solveRates = append(solveRates, solveRate)

		// ── Reflect (all but the last generation) ────────────────────────
		var nextPrompt string
		var reflectionText *string

		if genIdx < run.Generations-1 {
			stats := reflector.GenerationStats{
				GenIndex:      genIdx,
				SolveRate:     solveRate,
				MeanGuesses:   meanGuesses,
				MeanInfoGain:  meanInfoGain,
				ViolationRate: violationRate,
				FailedSamples: failedSamples,
			}
			np, refText, ok, refUsage, refErr := refl.Reflect(ctx, currentPrompt, stats)
			totalTokens += refUsage.InputTokens + refUsage.OutputTokens
			if refErr != nil {
				log.Printf("run %d gen %d: reflector error: %v", runID, genIdx, refErr)
			}
			if !ok {
				o.Hub.Publish(runID, Event{
					Type:    "error",
					Message: "reflector output missing PROMPT delimiters; reusing previous prompt",
				})
			}
			nextPrompt = np
			reflectionText = &refText
		} else {
			// Final generation — no reflection.
			nextPrompt = currentPrompt
		}

		// ── Persist generation stats ──────────────────────────────────────
		gen.SolveRate = &solveRate
		gen.MeanGuesses = &meanGuesses
		gen.MeanInfoGain = &meanInfoGain
		gen.ViolationRate = &violationRate
		gen.TokensUsed = totalTokens
		gen.ReflectionText = reflectionText
		if err := o.Store.UpdateGenerationStats(ctx, gen); err != nil {
			log.Printf("run %d gen %d: update stats: %v", runID, genIdx, err)
		}

		// ── Emit gen_end SSE ──────────────────────────────────────────────
		o.Hub.Publish(runID, Event{
			Type:          "gen_end",
			GenIndex:      genIdx,
			SolveRate:     &solveRate,
			MeanGuesses:   &meanGuesses,
			MeanInfoGain:  &meanInfoGain,
			ViolationRate: &violationRate,
			Prompt:        nextPrompt,
		})

		currentPrompt = nextPrompt

		// Yield briefly to avoid starving other goroutines.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	convergence := Convergence(solveRates)
	_ = o.Store.UpdateRunStatus(ctx, runID, "completed")
	o.Hub.Publish(runID, Event{
		Type:        "run_end",
		RunID:       runID,
		Status:      "completed",
		Convergence: convergence,
	})

	return nil
}

// llmGameResult holds the outcome of one LLM-played game.
type llmGameResult struct {
	won        bool
	numGuesses int
	infoGain   float64
	violations int
	tokensUsed int
	transcript string // formatted for failed-game samples
}

// playLLMGame plays a full game with the LLM agent, persists it, and emits SSE events.
func (o *Orchestrator) playLLMGame(ctx context.Context, a *agent.Agent, runID int64, genIdx int, prompt, answer string) (llmGameResult, error) {
	g := wordle.NewGame(answer)
	storeGame := &store.Game{
		RunID:     runID,
		GenIndex:  genIdx,
		Answer:    answer,
		AgentType: "llm",
	}
	gameID, err := o.Store.CreateGame(ctx, storeGame)
	if err != nil {
		return llmGameResult{}, fmt.Errorf("create game: %w", err)
	}
	storeGame.ID = gameID

	var (
		totalInfoGain float64
		totalViolations int
		totalTokens   int
		transcriptLines []string
	)

	// Build initial candidate list.
	candidates := make([]string, len(o.Lists.Answers))
	copy(candidates, o.Lists.Answers)

	for !g.IsOver() {
		turnIdx := len(g.Guesses)
		beforeCount := len(candidates)

		guess, reasoning, violation, usage, err := a.NextGuess(ctx, prompt, g)
		if err != nil {
			// Fall back and continue rather than abort the run.
			log.Printf("game %d turn %d: agent error: %v", gameID, turnIdx, err)
			if len(candidates) > 0 {
				guess = candidates[0]
			} else {
				guess = "crane"
			}
			violation = true
		}
		totalTokens += usage.InputTokens + usage.OutputTokens
		if violation {
			totalViolations++
		}

		fb := g.AddGuess(guess)
		candidates = wordle.FilterCandidates(candidates, guess, fb)
		afterCount := len(candidates)
		bits := wordle.InfoGainBits(beforeCount, afterCount)
		totalInfoGain += bits

		fbStr := fb.String()
		transcriptLines = append(transcriptLines, fmt.Sprintf("  Turn %d: %s → %s", turnIdx+1, guess, fbStr))

		// Persist guess.
		var reasoningPtr *string
		if reasoning != "" {
			reasoningPtr = &reasoning
		}
		gu := &store.Guess{
			GameID:        gameID,
			TurnIndex:     turnIdx,
			Guess:         guess,
			Feedback:      fbStr,
			InfoGainBits:  bits,
			ReasoningText: reasoningPtr,
		}
		if _, err := o.Store.CreateGuess(ctx, gu); err != nil {
			log.Printf("game %d: create guess: %v", gameID, err)
		}

		// Emit guess SSE event.
		o.Hub.Publish(runID, Event{
			Type:     "guess",
			GameID:   gameID,
			GenIndex: genIdx,
			Turn:     turnIdx,
			Guess:    guess,
			Feedback: fbStr,
			InfoGain: bits,
		})
	}

	// Update game row with final results.
	storeGame.Won = g.Won
	storeGame.NumGuesses = len(g.Guesses)
	storeGame.InfoGainTotal = totalInfoGain
	storeGame.Violations = totalViolations
	if err := o.Store.UpdateGame(ctx, storeGame); err != nil {
		log.Printf("game %d: update game: %v", gameID, err)
	}

	// Emit game_end SSE event.
	o.Hub.Publish(runID, Event{
		Type:       "game_end",
		GameID:     gameID,
		GenIndex:   genIdx,
		Won:        boolPtr(g.Won),
		NumGuesses: len(g.Guesses),
		Answer:     answer,
	})

	transcript := fmt.Sprintf("Word: %s\n%s", answer, joinLines(transcriptLines))

	return llmGameResult{
		won:        g.Won,
		numGuesses: len(g.Guesses),
		infoGain:   totalInfoGain,
		violations: totalViolations,
		tokensUsed: totalTokens,
		transcript: transcript,
	}, nil
}

func joinLines(lines []string) string {
	result := ""
	for _, l := range lines {
		result += l + "\n"
	}
	return result
}

// playBaselineGame plays a full game with a baseline player and persists it.
func (o *Orchestrator) playBaselineGame(ctx context.Context, runID int64, genIdx int, answer string, player baselines.Player) error {
	g := wordle.NewGame(answer)
	storeGame := &store.Game{
		RunID:     runID,
		GenIndex:  genIdx,
		Answer:    answer,
		AgentType: player.Name(),
	}
	gameID, err := o.Store.CreateGame(ctx, storeGame)
	if err != nil {
		return fmt.Errorf("create game: %w", err)
	}
	storeGame.ID = gameID

	candidates := make([]string, len(o.Lists.Answers))
	copy(candidates, o.Lists.Answers)

	var totalInfoGain float64

	for !g.IsOver() {
		turnIdx := len(g.Guesses)
		beforeCount := len(candidates)

		guess := player.Guess(g, candidates)
		if guess == "" {
			break
		}

		fb := g.AddGuess(guess)
		candidates = wordle.FilterCandidates(candidates, guess, fb)
		afterCount := len(candidates)
		bits := wordle.InfoGainBits(beforeCount, afterCount)
		totalInfoGain += bits

		fbStr := fb.String()
		gu := &store.Guess{
			GameID:       gameID,
			TurnIndex:    turnIdx,
			Guess:        guess,
			Feedback:     fbStr,
			InfoGainBits: bits,
		}
		if _, err := o.Store.CreateGuess(ctx, gu); err != nil {
			log.Printf("baseline game %d: create guess: %v", gameID, err)
		}
	}

	storeGame.Won = g.Won
	storeGame.NumGuesses = len(g.Guesses)
	storeGame.InfoGainTotal = totalInfoGain
	if err := o.Store.UpdateGame(ctx, storeGame); err != nil {
		log.Printf("baseline game %d: update game: %v", gameID, err)
	}

	return nil
}

// sampleWords returns a deterministic sample of n words from answers using
// a seeded Fisher-Yates shuffle (ARCHITECTURE.md §9.7).
func sampleWords(answers []string, seed int64, n int) []string {
	if n <= 0 || len(answers) == 0 {
		return nil
	}
	sample := make([]string, len(answers))
	copy(sample, answers)

	rng := rand.New(rand.NewPCG(uint64(seed), 0))
	rng.Shuffle(len(sample), func(i, j int) {
		sample[i], sample[j] = sample[j], sample[i]
	})

	if n >= len(sample) {
		return sample
	}
	return sample[:n]
}

// Convergence classifies a run from its completed generations' solve rates.
// Returns "improving" | "oscillating" | "stable" per ARCHITECTURE.md §9.6.
func Convergence(solveRates []float64) string {
	if len(solveRates) < 3 {
		return "improving"
	}

	// Use the last 3 solve rates.
	rates := solveRates[len(solveRates)-3:]
	g1, g2, g3 := rates[0], rates[1], rates[2]

	maxRate := g1
	if g2 > maxRate {
		maxRate = g2
	}
	if g3 > maxRate {
		maxRate = g3
	}

	minRate := g1
	if g2 < minRate {
		minRate = g2
	}
	if g3 < minRate {
		minRate = g3
	}

	if maxRate-minRate < 0.02 {
		return "stable"
	}

	d1 := g2 - g1
	d2 := g3 - g2
	if d1 != 0 && d2 != 0 && (d1 > 0) != (d2 > 0) {
		return "oscillating"
	}

	return "improving"
}

// heartbeatTicker is used in the SSE handler (exported so main.go can use it).
var HeartbeatInterval = 15 * time.Second
