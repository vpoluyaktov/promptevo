# promptevo — Architecture

> **Authoritative spec.** This document is the single source of truth for the
> backend, frontend, QA, and DevOps agents. Implement against this file. If a
> requirement is ambiguous or missing, ask the Architect and this document will
> be updated — do not invent divergent behavior.

---

## 1. Overview

**promptevo** is a research web app in which an LLM *agent* plays Wordle, then
*self-reflects* and rewrites its own strategy prompt after each generation of
games. Over successive generations the agent's prompt evolves; researchers use
the UI to compare self-improvement dynamics across LLM models served through
**OpenRouter** (OpenAI-compatible API).

### Design goals

- **Reproducibility** — every run is seeded; word samples and game order are
  deterministic given `(seed, word_sample_size)`.
- **Observability** — a run streams live events (guesses, game ends, generation
  ends) over SSE so the UI animates progress in real time.
- **Comparability** — fixed metrics (solve rate, mean guesses, mean information
  gain, violation rate, convergence) let researchers compare models head-to-head.
- **Self-containment** — single Go binary + embedded SQLite (CGO-free), static
  React bundle served by nginx, all wired with Docker Compose.

### Technology stack

| Layer        | Choice |
|--------------|--------|
| Language     | Go **1.23+** |
| HTTP router  | `github.com/go-chi/chi/v5` |
| Database     | `modernc.org/sqlite` (pure-Go, **CGO-free**) on a Docker volume |
| LLM gateway  | OpenRouter (`https://openrouter.ai/api/v1/chat/completions`, OpenAI-compatible) |
| Frontend     | React 18 + TypeScript + Vite, **Recharts**, **light theme** |
| Web server   | nginx (serves static bundle, reverse-proxies `/api`) |
| Infra        | Docker Compose (2 services + 1 volume) |

> **Routing note (chi, not net/http ServeMux):** we use `chi/v5`, so the Go 1.22
> `GET /{$}` exact-match caveat does **not** apply. chi matches routes by
> registered method + pattern and returns `405 Method Not Allowed` on method
> mismatch automatically via `MethodNotAllowed`. There is no catch-all index
> route in the Go service — the SPA shell is served by **nginx**, and the Go
> binary serves only `/api/*` (plus `/healthz`). See §8.

---

## 2. System Diagram

```
                          ┌──────────────────────────────────────────┐
                          │                Browser (SPA)             │
                          │   React 18 + TS + Vite + Recharts        │
                          │   Views: Runs · NewRun · RunDetail       │
                          └───────────────┬──────────────────────────┘
                                          │  HTTP (JSON) + SSE
                                          ▼
                ┌───────────────────────────────────────────────────┐
                │                 nginx (service: web)              │
                │   - serves static React bundle (/, /assets/*)     │
                │   - proxies /api/*  ->  api:8080                   │
                │   - proxies SSE (proxy_buffering off)             │
                └───────────────┬───────────────────────────────────┘
                                │  http://api:8080
                                ▼
        ┌───────────────────────────────────────────────────────────────┐
        │                     Go service (service: api)                 │
        │                                                               │
        │  cmd/server ── chi router ── handlers (REST + SSE)            │
        │        │                                                      │
        │        ▼                                                      │
        │  internal/experiment  (orchestrates a run, goroutine)        │
        │        │      │            │             │                    │
        │        ▼      ▼            ▼             ▼                    │
        │   agent   reflector    wordle        baselines               │
        │     │         │           (scoring,                          │
        │     ▼         ▼            info gain)                         │
        │   llm.Client (OpenRouter HTTP)                               │
        │        │                                                      │
        │        ▼                                                      │
        │   internal/store  ── modernc.org/sqlite ──► /data/promptevo.db│
        └───────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
                                ┌───────────────────┐
                                │  Docker volume    │
                                │  promptevo-data   │
                                └───────────────────┘

  OpenRouter (external)  ◄──── llm.Client (HTTPS, Bearer OPENROUTER_API_KEY)
```

---

## 3. Go Package Layout

Go module name: **`promptevo`**

```
promptevo/
├── ARCHITECTURE.md              # This file — authoritative spec
├── go.mod                       # module promptevo, go 1.23
├── go.sum                       # generated by go mod tidy
│
├── cmd/
│   └── server/
│       └── main.go              # Entrypoint: load config → open store → run migrations
│                                #   → build router → http.Server → graceful shutdown
├── internal/
│   ├── wordle/
│   │   └── wordle.go            # Pure game logic: ScoreGuess, Feedback, Game,
│   │                            #   word-list loading, candidate filtering, info gain
│   ├── agent/
│   │   └── agent.go             # LLM Wordle player: builds the per-turn prompt from
│   │                            #   game state + strategy prompt, calls llm.Client,
│   │                            #   parses the guess, enforces validity (counts violations)
│   ├── reflector/
│   │   └── reflector.go         # Post-generation self-reflection: builds reflection
│   │                            #   prompt from generation stats, calls llm.Client,
│   │                            #   parses ---PROMPT_START---/---PROMPT_END--- block
│   ├── baselines/
│   │   └── baselines.go         # Non-LLM reference players (random, frequency, entropy)
│   │                            #   for comparison against the evolving agent
│   ├── llm/
│   │   └── llm.go               # Client interface + OpenRouter implementation
│   ├── store/
│   │   └── store.go             # Store interface + sqliteStore implementation
│   └── experiment/
│       └── experiment.go        # Run orchestrator: loops generations × games,
│                                #   drives agent + reflector, persists rows,
│                                #   publishes SSE events via a Hub
├── migrations/
│   ├── 001_initial.up.sql       # Create runs, generations, games, guesses
│   └── 001_initial.down.sql     # Drop all four tables
├── data/
│   ├── answers.txt              # Wordle answer pool (one lowercase word/line) — PLACEHOLDER*
│   └── guesses.txt              # Valid-guess pool (superset of answers)        — PLACEHOLDER*
└── frontend/                    # React app (scaffolded by Frontend dev with Vite)
```

> **\*Word-list placeholders.** `data/answers.txt` and `data/guesses.txt` are
> currently filled with a curated set of real 5-letter dictionary words
> (584 answers, 4667 guesses; `answers ⊆ guesses`). These are **placeholders**.
> The Backend developer must replace them with the canonical NYT Wordle lists
> (**2309** answers, **10657** valid guesses) before benchmarking. The file
> format and loading code do not change — only the contents.

### Package responsibilities & key types

#### `internal/wordle`
Pure, dependency-free game logic. No I/O except `LoadWordLists`.

- `type TileResult int` — `Gray=0`, `Yellow=1`, `Green=2`.
- `type Feedback [5]TileResult`; `Feedback.String()` → 5-char `G/Y/X` code.
- `type Game struct` — holds `Answer string`, `Guesses []string`,
  `Feedbacks []Feedback`, `Won bool`, `MaxTurns int` (=6).
- `func ScoreGuess(guess, answer string) Feedback` — see §9 for the exact
  duplicate-letter algorithm and edge cases.
- `type WordLists struct { Answers []string; Guesses map[string]struct{} }`
- `func LoadWordLists(answersPath, guessesPath string) (*WordLists, error)`
- `func (wl *WordLists) IsValidGuess(word string) bool`
- `func FilterCandidates(candidates []string, guess string, fb Feedback) []string`
  — returns the subset of `candidates` consistent with `fb` for `guess`
  (i.e., `ScoreGuess(guess, c) == fb`). Basis for information gain.
- `func InfoGainBits(before, after int) float64` — `log2(before/after)`; see §9.

#### `internal/llm`
LLM transport abstraction (interface in §6-ish below, full def here).

```go
package llm

type Message struct {
    Role    string `json:"role"`    // "system" | "user" | "assistant"
    Content string `json:"content"`
}

type CompletionRequest struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    Temperature float64   `json:"temperature"`
    MaxTokens   int       `json:"max_tokens,omitempty"`
}

type CompletionResponse struct {
    Content      string `json:"content"`
    InputTokens  int    `json:"input_tokens"`
    OutputTokens int    `json:"output_tokens"`
}

type Client interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
```

The OpenRouter implementation (`openRouterClient`) POSTs to
`{base}/chat/completions`, sends `Authorization: Bearer <OPENROUTER_API_KEY>`,
maps `choices[0].message.content` → `Content` and `usage.prompt_tokens` /
`usage.completion_tokens` → token counts. It retries transient 429/5xx with
exponential backoff (3 attempts) and honors `ctx` cancellation.

#### `internal/agent`
Turns game state + the current strategy prompt into a single guess.

- `type Agent struct { Client llm.Client; Lists *wordle.WordLists; Model string; Temperature float64 }`
- `func (a *Agent) NextGuess(ctx, strategyPrompt string, g *wordle.Game) (guess string, reasoning string, violation bool, usage TokenUsage, err error)`
  - Builds the per-turn user message (board so far, confirmed/excluded letters).
  - Calls the LLM, extracts a 5-letter token from the reply (see parse rules §9.4).
  - **Violation** = the parsed guess is not a valid word, not 5 letters, or
    contradicts known hard constraints. On violation the agent falls back to a
    deterministic valid candidate (first remaining candidate) so the game
    proceeds, and `violation=true` is recorded.

#### `internal/reflector`
Rewrites the strategy prompt between generations.

- `type Reflector struct { Client llm.Client; Model string; Temperature float64 }`
- `func (r *Reflector) Reflect(ctx, currentPrompt string, stats GenerationStats) (newPrompt string, reflection string, ok bool, usage TokenUsage, err error)`
  - `ok=false` when the delimited block can't be parsed → caller reuses
    `currentPrompt` and logs an `error` SSE event. See §9.5 and §11.

#### `internal/baselines`
Deterministic, non-LLM players for reference curves.

- `RandomPlayer` — picks a uniformly random remaining candidate (seeded).
- `FrequencyPlayer` — picks the candidate maximizing positional letter frequency.
- `EntropyPlayer` — picks the guess maximizing expected information gain.
- Common interface: `type Player interface { Guess(g *wordle.Game, candidates []string) string }`.
- Used to populate `games.agent_type` rows (`'random'|'frequency'|'entropy'`)
  alongside `'llm'`, so the UI can overlay baseline solve rates.

#### `internal/store`
Persistence. Interface + sqlite implementation. See §5.

#### `internal/experiment`
Run orchestrator. One run = a goroutine that:
1. Samples `word_sample_size` answers from `WordLists.Answers` (seeded shuffle).
2. For each generation `g` in `0..generations-1`:
   - Plays `games_per_gen` games with the current strategy prompt (the same
     seeded word sample each generation, so generations are comparable).
   - Persists each game + its guesses; emits `guess` / `game_end` SSE events.
   - Aggregates `GenerationStats`, writes the `generations` row, emits `gen_end`.
   - If `g < generations-1`: calls the reflector to produce the next prompt.
3. Marks the run `completed` (or `failed`), emits `run_end`.

- `type Hub` — fan-out of SSE events keyed by `runID` (subscribe/publish/unsubscribe).
- `type Orchestrator struct { Store store.Store; Agent *agent.Agent; Reflector *reflector.Reflector; Lists *wordle.WordLists; Hub *Hub }`
- `func (o *Orchestrator) StartRun(runID int64)` — launches the goroutine.

---

## 4. REST API

Base path: **`/api`** (the Go service mounts the router at `/api`; nginx proxies
`/api/*` → `api:8080`). All request/response bodies are JSON
(`Content-Type: application/json`) unless noted. Errors use the shape:

```json
{ "error": "human readable message" }
```

| Method | Path                        | Description |
|--------|-----------------------------|-------------|
| GET    | `/healthz`                  | Liveness probe (not under `/api`). |
| GET    | `/api/models`               | List selectable OpenRouter models. |
| GET    | `/api/runs`                 | List all runs (newest first). |
| POST   | `/api/runs`                 | Create + start a run. |
| GET    | `/api/runs/{id}`            | Run detail incl. generations. |
| GET    | `/api/runs/{id}/generations`| Generations for a run. |
| GET    | `/api/runs/{id}/games`      | Games for a run (optional `?gen=` filter). |
| GET    | `/api/games/{id}/guesses`   | Guesses (turns) for one game. |
| GET    | `/api/runs/{id}/stream`     | **SSE** live event stream (see §7). |
| DELETE | `/api/runs/{id}`            | Delete a run and all child rows. |

### 4.0 `GET /healthz`
**200** `{"status":"ok"}`. No body in request.

### 4.1 `GET /api/models`
Returns the hardcoded selectable model list (player & reflector pickers).

**Response 200**
```json
{
  "models": [
    "anthropic/claude-3.5-sonnet",
    "anthropic/claude-3-haiku",
    "openai/gpt-4o",
    "openai/gpt-4o-mini",
    "google/gemini-2.0-flash",
    "google/gemini-1.5-pro",
    "meta-llama/llama-3.3-70b-instruct",
    "mistralai/mistral-large"
  ]
}
```

### 4.2 `GET /api/runs`
**Response 200** — newest first; empty list returns `{"runs":[]}` (never `null`).
```json
{
  "runs": [
    {
      "id": 7,
      "createdAt": "2026-06-05T14:03:11Z",
      "playerModel": "openai/gpt-4o-mini",
      "reflectorModel": "anthropic/claude-3.5-sonnet",
      "temperature": 0.7,
      "seed": 42,
      "generations": 5,
      "gamesPerGen": 20,
      "wordSampleSize": 50,
      "status": "running"
    }
  ]
}
```
`status` ∈ `pending | running | completed | failed`.

### 4.3 `POST /api/runs`
Creates a run row (`status:"pending"`), launches the orchestrator goroutine
(transitions to `running`), and returns the created run immediately.

**Request**
```json
{
  "playerModel": "openai/gpt-4o-mini",
  "reflectorModel": "anthropic/claude-3.5-sonnet",
  "temperature": 0.7,
  "seed": 42,
  "generations": 5,
  "gamesPerGen": 20,
  "wordSampleSize": 50,
  "includeBaselines": true
}
```

| Field            | Type    | Required | Default | Constraints |
|------------------|---------|----------|---------|-------------|
| `playerModel`    | string  | yes      | —       | must be in `/api/models` |
| `reflectorModel` | string  | yes      | —       | must be in `/api/models` |
| `temperature`    | number  | no       | `0.7`   | `0.0–2.0` |
| `seed`           | integer | no       | `42`    | any int64 |
| `generations`    | integer | yes      | —       | `1–50` |
| `gamesPerGen`    | integer | yes      | —       | `1–500` |
| `wordSampleSize` | integer | yes      | —       | `1–`len(answers) |
| `includeBaselines`| boolean| no       | `false` | run random/frequency/entropy in gen 0 for reference |

**Response 201**
```json
{
  "id": 7,
  "createdAt": "2026-06-05T14:03:11Z",
  "playerModel": "openai/gpt-4o-mini",
  "reflectorModel": "anthropic/claude-3.5-sonnet",
  "temperature": 0.7,
  "seed": 42,
  "generations": 5,
  "gamesPerGen": 20,
  "wordSampleSize": 50,
  "status": "running"
}
```

**Errors**
- `400` — missing required field, unknown model, or out-of-range value:
  `{"error":"generations must be between 1 and 50"}`
- `503` — `OPENROUTER_API_KEY` not configured: `{"error":"LLM gateway not configured"}`

### 4.4 `GET /api/runs/{id}`
**Response 200** — run plus its generation summaries.
```json
{
  "id": 7,
  "createdAt": "2026-06-05T14:03:11Z",
  "playerModel": "openai/gpt-4o-mini",
  "reflectorModel": "anthropic/claude-3.5-sonnet",
  "temperature": 0.7,
  "seed": 42,
  "generations": 5,
  "gamesPerGen": 20,
  "wordSampleSize": 50,
  "status": "completed",
  "convergence": "stable",
  "generationsData": [
    {
      "genIndex": 0,
      "promptText": "You are an expert Wordle player...",
      "promptLen": 412,
      "reflectionText": "The agent wasted guesses re-using excluded letters...",
      "solveRate": 0.60,
      "meanGuesses": 4.30,
      "meanInfoGain": 6.81,
      "violationRate": 0.08,
      "tokensUsed": 51234
    }
  ]
}
```
`convergence` ∈ `improving | oscillating | stable` (see §9.6); it is `"improving"`
when fewer than 3 generations have completed.

**Errors** — `404` `{"error":"run not found"}`.

### 4.5 `GET /api/runs/{id}/generations`
**Response 200**
```json
{ "generations": [ { "genIndex": 0, "promptText": "...", "promptLen": 412,
  "reflectionText": "...", "solveRate": 0.60, "meanGuesses": 4.30,
  "meanInfoGain": 6.81, "violationRate": 0.08, "tokensUsed": 51234 } ] }
```

### 4.6 `GET /api/runs/{id}/games`
Optional query `?gen=<genIndex>` filters to one generation. No filter → all games.

**Response 200**
```json
{
  "games": [
    {
      "id": 101,
      "genIndex": 0,
      "answer": "crane",
      "won": true,
      "numGuesses": 4,
      "infoGainTotal": 9.12,
      "violations": 0,
      "agentType": "llm"
    }
  ]
}
```

### 4.7 `GET /api/games/{id}/guesses`
**Response 200** — ordered by `turnIndex` ascending.
```json
{
  "guesses": [
    {
      "id": 5001,
      "turnIndex": 0,
      "guess": "slate",
      "feedback": "XXYXG",
      "infoGainBits": 4.10,
      "reasoningText": "Starting with a common-letter opener..."
    }
  ]
}
```
`feedback` is the 5-char `G/Y/X` code (Green/Yellow/Gray). Empty game → `{"guesses":[]}`.

### 4.8 `DELETE /api/runs/{id}`
Deletes the run and **all** child `generations`, `games`, `guesses` rows in a
single transaction. **Response 200** `{"deleted": true}`. Unknown id → `404`.

### 4.9 `GET /api/runs/{id}/stream`
Server-Sent Events. See §7 for the full event catalog and framing.

---

## 5. Database Schema

SQLite file at `/data/promptevo.db` (Docker volume). Migrations in `migrations/`
applied at startup (forward-only `001_initial.up.sql`). Pragmas set on open:
`journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`.

### Tables

**`runs`**

| Column            | Type     | Notes |
|-------------------|----------|-------|
| `id`              | INTEGER PK AUTOINCREMENT | |
| `created_at`      | DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP | UTC |
| `player_model`    | TEXT NOT NULL | OpenRouter id |
| `reflector_model` | TEXT NOT NULL | OpenRouter id |
| `temperature`     | REAL NOT NULL DEFAULT 0.7 | |
| `seed`            | INTEGER NOT NULL | |
| `generations`     | INTEGER NOT NULL | configured count |
| `games_per_gen`   | INTEGER NOT NULL | |
| `word_sample_size`| INTEGER NOT NULL | |
| `status`          | TEXT NOT NULL DEFAULT 'pending' | pending/running/completed/failed |
| `config_json`     | TEXT NOT NULL DEFAULT '{}' | full request echo (incl. `includeBaselines`) |

**`generations`**

| Column           | Type   | Notes |
|------------------|--------|-------|
| `id`             | INTEGER PK AUTOINCREMENT | |
| `run_id`         | INTEGER NOT NULL REFERENCES runs(id) | |
| `gen_index`      | INTEGER NOT NULL | 0-based |
| `prompt_text`    | TEXT NOT NULL | strategy prompt used this generation |
| `prompt_len`     | INTEGER NOT NULL | `len([]rune(prompt_text))` |
| `reflection_text`| TEXT     | reflector reasoning; NULL for the last gen (no reflection) |
| `solve_rate`     | REAL     | fraction won `[0,1]` |
| `mean_guesses`   | REAL     | mean guesses over games this gen |
| `mean_info_gain` | REAL     | mean total info-gain bits per game |
| `violation_rate` | REAL     | mean violations per game |
| `tokens_used`    | INTEGER NOT NULL DEFAULT 0 | player+reflector tokens this gen |

**`games`**

| Column           | Type   | Notes |
|------------------|--------|-------|
| `id`             | INTEGER PK AUTOINCREMENT | |
| `run_id`         | INTEGER NOT NULL REFERENCES runs(id) | |
| `gen_index`      | INTEGER NOT NULL | denormalized for fast filtering |
| `answer`         | TEXT NOT NULL | lowercase 5-letter |
| `won`            | INTEGER NOT NULL DEFAULT 0 | 0/1 boolean |
| `num_guesses`    | INTEGER NOT NULL DEFAULT 0 | 1–6 |
| `info_gain_total`| REAL NOT NULL DEFAULT 0 | sum of per-turn bits |
| `violations`     | INTEGER NOT NULL DEFAULT 0 | invalid/contradictory guesses |
| `agent_type`     | TEXT NOT NULL DEFAULT 'llm' | llm/random/frequency/entropy |

**`guesses`**

| Column          | Type   | Notes |
|-----------------|--------|-------|
| `id`            | INTEGER PK AUTOINCREMENT | |
| `game_id`       | INTEGER NOT NULL REFERENCES games(id) | |
| `turn_index`    | INTEGER NOT NULL | 0-based |
| `guess`         | TEXT NOT NULL | lowercase 5-letter |
| `feedback`      | TEXT NOT NULL | 5-char G/Y/X code |
| `info_gain_bits`| REAL NOT NULL DEFAULT 0 | bits gained this turn |
| `reasoning_text`| TEXT   | model's chain-of-thought snippet (nullable) |

### Recommended indexes (add in `001_initial.up.sql`)
```sql
CREATE INDEX IF NOT EXISTS idx_generations_run ON generations(run_id, gen_index);
CREATE INDEX IF NOT EXISTS idx_games_run       ON games(run_id, gen_index);
CREATE INDEX IF NOT EXISTS idx_guesses_game    ON guesses(game_id, turn_index);
```

### Document ID strategy
SQLite autoincrement integer PKs. No natural keys. `run_id` / `game_id` foreign
keys link children to parents. Deletes cascade **in application code** within a
transaction (SQLite FK `ON DELETE CASCADE` is not declared in the migration; the
store performs ordered deletes — see §6 `DeleteRun`).

---

## 6. Store Interface

```go
package store

import "context"

type Store interface {
    // lifecycle
    Migrate(ctx context.Context) error
    Close() error

    // runs
    CreateRun(ctx context.Context, r *Run) (int64, error)
    GetRun(ctx context.Context, id int64) (*Run, error)
    ListRuns(ctx context.Context) ([]*Run, error)
    UpdateRunStatus(ctx context.Context, id int64, status string) error
    DeleteRun(ctx context.Context, id int64) error   // tx: guesses → games → generations → run

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
```

### Method behavior

- `Migrate` — applies `migrations/001_initial.up.sql` (idempotent via
  `IF NOT EXISTS`). Called once at startup.
- `CreateRun` — inserts a `runs` row, returns new id. `created_at` defaulted by DB.
- `GetRun` — returns `(nil, ErrNotFound)` when absent so handlers map to `404`.
- `ListRuns` — newest first (`ORDER BY id DESC`); returns empty slice, not nil.
- `UpdateRunStatus` — sets `status`; used at `running`/`completed`/`failed`.
- `DeleteRun` — **bulk multi-row delete**. Runs inside a single
  `BEGIN…COMMIT` transaction deleting children before parents:
  `DELETE FROM guesses WHERE game_id IN (SELECT id FROM games WHERE run_id=?)`,
  then `games`, then `generations`, then the `runs` row. On any error the tx
  rolls back. *(SQLite has no Firestore `BulkWriter`; the equivalent
  "atomic bulk write" primitive here is a transaction — use one, never
  per-row autocommit deletes.)*
- `CreateGeneration` / `UpdateGenerationStats` — insert the prompt row at gen
  start, then patch stats (`solve_rate`, `mean_*`, `violation_rate`,
  `reflection_text`, `tokens_used`) at gen end.
- `ListGames` — `genIndex` nil → all games for the run; non-nil → filtered.
- All list methods order by their natural ascending key (`gen_index`,
  `id`, `turn_index`).

### Data structs (store package)
Field names map to DB columns; JSON tags match the API wire format in §4.
`*string` / `*float64` are used for nullable columns (`reflection_text`,
`solve_rate`, etc.) so "not yet computed" is distinct from `0`.

---

## 7. SSE — `GET /api/runs/{id}/stream`

Content negotiation:
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`
- `X-Accel-Buffering: no` (and nginx `proxy_buffering off` for this location)

Framing: each event is emitted as a single SSE `data:` line containing a JSON
object, terminated by a blank line. The server flushes after every event
(`http.Flusher`). A heartbeat comment (`: ping\n\n`) is sent every 15s to keep
the connection alive. If the run is already finished when a client connects,
the server replays a terminal `run_end` and closes.

All events share an envelope `{"type": "...", ...}`. The `type` values and shapes:

**`guess`** — emitted after each scored guess:
```json
{"type":"guess","gameId":101,"genIndex":0,"turn":0,"guess":"slate","feedback":"XXYXG","infoGain":4.10}
```

**`game_end`** — emitted when a game finishes (win or 6-guess loss):
```json
{"type":"game_end","gameId":101,"genIndex":0,"won":true,"numGuesses":4,"answer":"crane"}
```

**`gen_end`** — emitted after a generation's games + reflection:
```json
{"type":"gen_end","genIndex":0,"solveRate":0.60,"meanGuesses":4.30,"meanInfoGain":6.81,"violationRate":0.08,"prompt":"You are an expert Wordle player..."}
```
The `prompt` field carries the **next** generation's prompt (the reflector's
output). For the final generation there is no reflection, so `prompt` repeats
the generation's own prompt.

**`run_end`** — terminal:
```json
{"type":"run_end","runId":7,"status":"completed","convergence":"stable"}
```

**`error`** — non-fatal warning (e.g., reflector parse failure → prompt reused)
or fatal run error (followed by a `run_end` with `status:"failed"`):
```json
{"type":"error","message":"reflector output missing PROMPT delimiters; reusing previous prompt"}
```

> **Client note:** events are newline-delimited JSON in the SSE `data:` field.
> The frontend uses the browser `EventSource` API on
> `/api/runs/{id}/stream` and switches on `event.type` after `JSON.parse`.

---

## 8. Frontend

React 18 + TypeScript + Vite. **Light theme only.** Charts via **Recharts**.
Built to a static bundle served by nginx; `/api` is proxied to the Go service.

### Routes / Views

| Route             | View         | Purpose |
|-------------------|--------------|---------|
| `/`               | `RunsList`   | Table of all runs (status, models, metrics), "New Run" button, row → detail. |
| `/runs/new`       | `NewRun`     | Form to configure + start a run (model pickers from `/api/models`). |
| `/runs/:id`       | `RunDetail`  | Live + historical view of one run. |

### Component structure

```
src/
├── main.tsx                 # Vite entry, router
├── App.tsx                  # layout shell (light theme), nav
├── api/
│   ├── client.ts            # fetch wrappers (typed)
│   └── types.ts             # Run, Generation, Game, Guess, SSEEvent unions
├── views/
│   ├── RunsList.tsx
│   ├── NewRun.tsx
│   └── RunDetail.tsx
├── components/
│   ├── RunForm.tsx          # controlled form; validates against §4.3 constraints
│   ├── MetricsChart.tsx     # Recharts line chart: solveRate / meanGuesses vs gen
│   ├── PromptDiff.tsx       # shows prompt evolution per generation (diff highlight)
│   ├── GameBoard.tsx        # 5×6 Wordle grid, colored tiles from feedback string
│   ├── LiveFeed.tsx         # consumes EventSource, animates guesses live
│   ├── ConvergenceBadge.tsx # improving / oscillating / stable pill
│   └── GameList.tsx         # per-generation game results table
└── hooks/
    └── useRunStream.ts      # wraps EventSource for /api/runs/:id/stream
```

### RunDetail behavior
- On mount: `GET /api/runs/:id` for the historical state, render metrics chart
  + per-generation prompt panels.
- If `status==="running"`: open `useRunStream(id)`; append `guess`/`game_end`
  to `LiveFeed`, update charts on `gen_end`, close on `run_end`.
- Tile colors (light theme): Green `#6aaa64`, Yellow `#c9b458`, Gray `#d3d6da`
  (text dark on gray). These match canonical Wordle on a light background.

### Metrics visualized
- Line chart: `solveRate` and `meanGuesses` per `genIndex`.
- Line chart: `meanInfoGain` per `genIndex`.
- Bar/overlay: baseline players (if `includeBaselines`) vs LLM at gen 0.
- `prompt_len` sparkline (does the prompt grow or shrink as it evolves?).

---

## 9. Algorithms, Logic & Edge Cases

### 9.1 Wordle scoring — `ScoreGuess(guess, answer string) Feedback`

**Two-pass algorithm with correct duplicate-letter handling:**

1. **Pass 1 (greens):** for each position `i` (0–4), if `guess[i] == answer[i]`,
   set `result[i]=Green` and decrement a per-letter `remaining` count built from
   `answer`. (Conceptually: `remaining[c] = count of c in answer`, then for each
   green, `remaining[guess[i]]--`.)
2. **Pass 2 (yellows/grays):** for each non-green position `i` **left to right**,
   if `remaining[guess[i]] > 0`, set `result[i]=Yellow` and `remaining[guess[i]]--`;
   otherwise `result[i]=Gray`.

This guarantees a letter is colored (green or yellow) **at most as many times as
it occurs in the answer**, with greens taking priority and yellows consumed
left-to-right.

**Worked edge cases (these become QA's golden tests):**

| guess  | answer | feedback | why |
|--------|--------|----------|-----|
| `crane`| `crane`| `GGGGG`  | all exact. |
| `slate`| `crane`| `XXGXG`  | a (pos2) and e (pos4) match exactly → Green; s,l,t absent. |
| `babes`| `abbey`| `YYGGX`  | pos2 b & pos3 e green; one b and the a yellow; s gray. |
| `speed`| `abide`| `XXYXY`  | first e Yellow (budget=1), second e Gray (budget spent), d Yellow. |
| `geese`| `eject`| `XYXXY`*` | answer has two e's: greens first then yellows consume the 2-budget. |
| `aaaaa`| `crane`| `XXGXX`  | only the positional-match a (pos2) is green; no other a budget remains. |
| `eevee`| `crane`| `YXXXX`  | answer has one e (pos4, not matched by any guess pos here)… see note. |

> Compute these with the reference algorithm; do **not** hand-wave. The
> `eevee/crane` and `geese/eject` rows are intentionally tricky — QA must derive
> the exact expected string by running `ScoreGuess` and freeze it as a golden
> value. The principle, not the memorized string, is the spec: *greens first,
> then yellows left-to-right bounded by remaining answer count.*

**Boundary / invalid input behavior:**
- Inputs are assumed to be **exactly 5 lowercase ASCII letters**. The word lists
  guarantee this; `agent` validates LLM output **before** calling `ScoreGuess`.
- If `len(guess) != 5 || len(answer) != 5`, `ScoreGuess` returns the zero
  `Feedback` (`XXXXX`) — callers must never rely on this; validate upstream.
- Empty string input → all-gray (`XXXXX`); this is a defensive default, not a
  meaningful score.
- Case: comparison is byte-wise; callers pass lowercase. The display layer
  uppercases for the UI; persistence stores lowercase.

> **Regex note:** the scorer and candidate filter use **no regular
> expressions** — they are array/count loops. There are therefore no
> zero-length-pattern match-count concerns to verify. (The template-standards
> regex rule is N/A for this project; documented here so the absence is
> deliberate, not an omission.)

### 9.2 Candidate filtering — `FilterCandidates`
Given the running candidate list (initially the full **answers** pool, sampled
or not), after a guess with feedback `fb`, keep candidate `c` iff
`ScoreGuess(guess, c) == fb`. This is the consistency filter used for both
information gain and the entropy baseline.

- Edge: if `candidates` is empty → returns empty (no panic).
- The true answer always survives (it is self-consistent), so `after >= 1`
  whenever the answer is in the pool.

### 9.3 Information gain — `InfoGainBits(before, after int) float64`
`bits = log2(before / after)` where `before` = candidate count **before** the
guess and `after` = count **after** filtering by the new feedback.

- `before == 0` → return `0` (no information definable; should not happen in a
  real game where the pool starts non-empty).
- `after == 0` → treat as `after = 1` to avoid `+Inf` (defensive; in practice
  `after >= 1` because the answer is always consistent).
- `after == before` → `0` bits (guess eliminated nothing).
- Guess equals the answer → `after == 1`, gain `= log2(before)`.
- Per-game `info_gain_total` = sum of per-turn bits; mean over games →
  `mean_info_gain`.

### 9.4 Agent guess parsing
The agent instructs the model to end its reply with `GUESS: <WORD>`. Parsing:
1. Find the last line/token matching `GUESS:\s*([A-Za-z]{5})` (case-insensitive),
   lowercase it.
2. If not found, scan the reply for the **last** standalone 5-letter alpha token.
3. If still none, or the word is not in `guesses.txt`, or it contradicts a
   confirmed green/known-absent constraint → **violation**: substitute the first
   remaining candidate and record `violation=true`.

The `reasoning_text` stored is the model reply with the final `GUESS:` line
stripped (truncated to a sane length, e.g. 2000 chars).

### 9.5 Reflector output parsing
The reflector must emit the rewritten prompt wrapped in delimiters:
```
---PROMPT_START---
<new strategy prompt text>
---PROMPT_END---
```
Parsing extracts the text strictly between the two delimiter lines, trimmed.

- If **either** delimiter is missing, or `START` appears after `END`, or the
  extracted text is empty → `ok=false`: the **previous** prompt is reused and an
  `error` SSE event is emitted (`"reflector output missing PROMPT delimiters;
  reusing previous prompt"`). The run continues.
- If multiple blocks appear, the **first** well-formed block wins.
- The extracted prompt is stored as the next generation's `prompt_text`;
  `prompt_len = len([]rune(text))`.

### 9.6 Convergence indicator
Computed over the last **3** completed generations' `solve_rate` values
`[g1, g2, g3]` (g3 = most recent):

- Fewer than 3 generations completed → **`improving`** (insufficient data).
- `max(g1,g2,g3) - min(g1,g2,g3) < 0.02` → **`stable`**.
- Else if the direction reverses — `sign(g2-g1) != sign(g3-g2)` and neither
  delta is zero → **`oscillating`**.
- Else → **`improving`** (monotonic trend beyond the stability band).

Exposed on `GET /api/runs/{id}` as `convergence` and in the `run_end` SSE event.

### 9.7 Seeded sampling
`word_sample_size` answers are chosen by a deterministic Fisher–Yates shuffle of
`WordLists.Answers` seeded with `run.seed`, then the first `word_sample_size`
taken. The **same** sample is replayed every generation so cross-generation
metric deltas reflect prompt quality, not word luck. Edge: `word_sample_size >=
len(answers)` → use the whole list (no error).

---

## 10. Initial Strategy Prompt (generation 0)

Stored verbatim as `generations.prompt_text` for `gen_index=0`. The Backend
developer places this constant in `internal/experiment` (e.g.
`DefaultStrategyPrompt`). Exact text:

```
You are an expert Wordle player. The goal is to find the hidden five-letter
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
GUESS: <WORD>
```

---

## 11. Reflector Prompt Contract

The reflector receives: the current strategy prompt, this generation's aggregate
stats (solve rate, mean guesses, violation rate, mean info gain), and a few
representative failed games (answer + the guess/feedback sequence). It is
instructed to diagnose weaknesses and produce an improved prompt, returning it
**only** inside the delimited block:

```
---PROMPT_START---
<improved strategy prompt>
---PROMPT_END---
```

Parsing per §9.5. On parse failure the previous prompt is reused (no crash).

---

## 12. Configuration (environment variables)

Loaded by `cmd/server/main.go` (env → struct, with defaults). The Go service:

| Variable             | Type   | Default                  | Description |
|----------------------|--------|--------------------------|-------------|
| `PORT`               | int    | `8080`                   | HTTP listen port. |
| `DB_PATH`            | string | `/data/promptevo.db`     | SQLite file path (on the volume). |
| `ANSWERS_PATH`       | string | `data/answers.txt`       | Answer word list. |
| `GUESSES_PATH`       | string | `data/guesses.txt`       | Valid-guess word list. |
| `OPENROUTER_API_KEY` | string | — (required for runs)    | Bearer token for OpenRouter. |
| `OPENROUTER_BASE_URL`| string | `https://openrouter.ai/api/v1` | Override for testing/mocks. |
| `LLM_TIMEOUT_SECONDS`| int    | `60`                     | Per-request LLM timeout. |
| `MAX_CONCURRENT_RUNS`| int    | `2`                      | Orchestrator concurrency cap. |
| `LOG_LEVEL`          | string | `info`                   | `debug|info|warn|error`. |

Precedence: explicit env var > built-in default. No config file. Missing
`OPENROUTER_API_KEY` is allowed at boot (health/list endpoints work) but
`POST /api/runs` returns `503` until it is set.

Frontend build-time: nginx proxies `/api` so the SPA uses **relative** URLs
(no `VITE_API_URL` needed in the Compose deployment).

---

## 13. Infrastructure & Deployment (Docker Compose)

This project deploys via **Docker Compose** — not GCP/Terraform/Cloud Run. The
GCP rules in the global config do not apply here.

### Services

```yaml
# docker-compose.yml (DevOps owns the final file)
services:
  api:
    build: { context: ., dockerfile: Dockerfile }   # multi-stage Go build
    environment:
      - PORT=8080
      - DB_PATH=/data/promptevo.db
      - OPENROUTER_API_KEY=${OPENROUTER_API_KEY}
    volumes:
      - promptevo-data:/data
    expose: ["8080"]              # internal only; not published to host
    restart: unless-stopped

  web:
    build: { context: ./frontend, dockerfile: Dockerfile }  # node build → nginx
    ports: ["8080:80"]            # host:container — UI entrypoint
    depends_on: [api]
    restart: unless-stopped

volumes:
  promptevo-data:
```

- **`api`** — Go binary. Multi-stage Dockerfile: `golang:1.23-alpine` build
  stage → minimal `alpine` (or `gcr.io/distroless/static` — CGO-free build
  permits it). Copies the binary, `migrations/`, and `data/` word lists. Runs as
  non-root. Listens on `:8080` inside the Compose network only.
- **`web`** — React build stage (`node:20-alpine`, `npm ci && npm run build`) →
  `nginx:alpine` serving `/usr/share/nginx/html`. nginx config:
  - `location / { try_files $uri /index.html; }` (SPA fallback).
  - `location /api/ { proxy_pass http://api:8080; }`
  - `location /healthz { proxy_pass http://api:8080; }`
  - SSE: inside the `/api/` block, `proxy_buffering off; proxy_read_timeout
    3600s; proxy_set_header Connection '';` so `EventSource` streams uninterrupted.
- **volume** `promptevo-data` — persists the SQLite DB across restarts.

### Build / run
```
export OPENROUTER_API_KEY=sk-or-...
docker compose up --build      # UI at http://localhost:8080
```

No CI/CD cloud pipeline is required by the spec. If DevOps adds CI, it should run
`go vet ./...`, `go test ./...`, `golangci-lint run ./...`, and a `docker compose
build` smoke check.

---

## 14. Design Decisions & Rationale

1. **chi over net/http ServeMux** — the team chose chi/v5 for URL params
   (`{id}`), middleware, and method-based 405s without the Go 1.22 `GET /{$}`
   exact-match footgun. There is no HTML catch-all in Go; the SPA shell is
   nginx's job, so the "index route swallows 405" problem cannot occur here.
2. **modernc.org/sqlite (CGO-free)** — pure-Go SQLite means a static binary, a
   trivially small distroless image, and no cross-compilation/CGO toolchain in
   CI. Trade-off: slightly slower than the C driver — irrelevant at this write
   volume.
3. **Transaction for bulk delete** — SQLite's atomic-bulk-write primitive is a
   transaction (there is no Firestore `BulkWriter` here). `DeleteRun` wraps
   ordered child-first deletes in one tx. Never per-row autocommit.
4. **SSE over WebSocket** — the data flow is one-way server→client; SSE is
   simpler, reconnects natively via `EventSource`, and survives the nginx proxy
   with `proxy_buffering off`.
5. **Same seeded word sample every generation** — isolates prompt quality from
   word luck so generation-over-generation deltas are meaningful.
6. **Interface-based DI** — `llm.Client` and `store.Store` are interfaces so QA
   can substitute mocks (a scripted `llm.Client` and an in-memory or temp-file
   store) for fast, deterministic, network-free tests.
7. **Violations recorded, not fatal** — an invalid LLM guess is substituted with
   a valid candidate and counted; the run never stalls, and `violation_rate`
   becomes a measurable quality signal of the evolving prompt.
8. **Light theme only** — research/readability context; canonical Wordle tile
   colors on a white background.

---

## 15. Testing Strategy (for QA)

- **`wordle.ScoreGuess`** — table-driven golden tests covering §9.1, especially
  duplicate-letter rows. Derive expected strings by running the algorithm, then
  freeze them.
- **`FilterCandidates` / `InfoGainBits`** — empty pool, single candidate,
  `after==before`, guess==answer.
- **Convergence** — <3 gens, stable band, oscillating, monotonic improving.
- **Reflector parse** — valid block, missing delimiter, reversed delimiters,
  empty block, multiple blocks.
- **Agent parse** — `GUESS:` line, bare token fallback, invalid word →
  violation + substitution.
- **Store** — CRUD round-trips and `DeleteRun` cascade (assert children gone) on
  a temp-file sqlite DB.
- **Handlers** — table-driven with a mock `Store`; assert exact JSON wire format
  from §4 (field names, `[]` not `null` for empties, status codes).
- **LLM** — mock `llm.Client` returning scripted completions; no network in tests.

---

## 16. Tooling / Versions

- Go **1.23+** is the target. **Note:** `go.mod` currently declares `go 1.22`
  because the scaffolding sandbox could not fetch the 1.23 toolchain. The
  Dockerfile build stage uses `golang:1.23-alpine` (a 1.23 toolchain builds a
  `go 1.22`-directive module without issue). Backend should bump the directive
  to `go 1.23` once on a machine with that toolchain and re-run `go mod tidy`.
- `github.com/go-chi/chi/v5` v5.1.0
- `modernc.org/sqlite` v1.33.1 (CGO-free)
- Local quality gate before commit: `go vet ./...`, `go test ./...`,
  `golangci-lint run ./...`.
- Frontend: Node 20, Vite 5, React 18, TypeScript 5, Recharts 2.

> This project does **not** use GitHub Actions GCP deploy workflows or Terraform;
> the global template-standards GitHub-Actions/Terraform version lists are not
> applicable. If a CI workflow is added, use `actions/checkout@v4` and
> `actions/setup-go@v5`.
