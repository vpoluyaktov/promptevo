# ARCHITECTURE — Analytics Feature (Run Detail → "Analysis" tab)

Authoritative implementation spec for the new **Analysis** tab on the Run Detail
page. Audience: the Backend Developer and Frontend Developer implement directly
from this document; the QA Engineer writes tests against the JSON shapes and edge
cases defined here. This document **extends** `ARCHITECTURE.md`; it does not
replace it.

The target end-user is an **ML academic scientist** studying prompt evolution in
LLM agents. Every chart ships with an `InfoPopup` that explains, in scientific
terms, what the metric measures, why it matters for prompt-evolution research, and
how to read good vs. bad values.

---

## 0. Scope and Design Principles

### 0.1 What is being added

1. One new backend endpoint: `GET /api/runs/{id}/analytics`.
2. Four new `Store` interface methods (+ their SQLite implementations + MockStore
   stubs) feeding the endpoint's aggregations.
3. A schema migration splitting per-generation token usage into **player** vs
   **reflector** tokens, and the `experiment.go` changes to populate them.
4. A new `Analysis` tab in `RunDetail.tsx` containing 9 chart cards.
5. A reusable `InfoPopup` component and a `ChartCard` wrapper.

### 0.2 Cross-cutting decisions (read before implementing)

- **Decision A — LLM games only.** Every analytics aggregation filters
  `games.agent_type = 'llm'`. Baseline games (`random`, `frequency`, `entropy`)
  are reference comparators on the existing Charts tab and would pollute
  prompt-evolution metrics. This filter is applied in **SQL** for every new query
  and in **Go** for the replay metric (§3.6). Rationale: the Analysis tab studies
  the evolving LLM agent, not the static baselines.
- **Decision B — single round-trip.** All 9 metrics are served by one endpoint
  except metric #6 (Prompt Edit Distance), which is computed client-side from
  `generationsData.promptText` already present in the `GET /api/runs/{id}`
  response. No second fetch is needed for #6. Rationale: prompt text is already on
  the client; recomputing Levenshtein server-side would duplicate payload.
- **Decision C — server computes statistics, client renders.** Wilson intervals
  (#1), win-distribution bucketing (#2), and candidate-replay (#5) are computed in
  Go and returned as final numbers. The client only pivots/plots. Rationale:
  keeps numerical correctness in one tested place (QA covers it with MockStore +
  table-driven tests) and keeps the React layer dumb.
- **Decision D — additive, backward-compatible schema.** Token split adds two
  columns via idempotent `ALTER TABLE` in `Migrate()` (same pattern as the
  existing `max_guesses` migration). The legacy `tokens_used` column is retained
  and kept equal to `player_tokens + reflector_tokens` for new rows. Old rows
  report `player_tokens = reflector_tokens = 0`; the frontend detects this and
  falls back to showing only the combined total.

---

## 1. Backend — New Endpoint

### 1.1 Route registration (`cmd/server/main.go`)

Add inside the protected group in `buildRoutes()`, next to the other
`/runs/{id}/...` routes:

```go
r.Get("/runs/{id}/analytics", s.handleGetAnalytics)
```

- **Auth:** protected (Bearer token), same as all other `/api/runs/*` routes.
- **chi routing note:** chi matches by method + path; a GET to a non-existent sub
  path returns 404 and a wrong method returns 405 natively, so no catch-all
  precedence concern exists here (this is a chi router, not Go 1.22 `ServeMux`).

### 1.2 Handler contract

`GET /api/runs/{id}/analytics`

| Case | Status | Body |
|------|--------|------|
| Success | `200` | `AnalyticsResponse` (§1.4) |
| Invalid id (`<= 0`, non-numeric) | `400` | `{"error":"invalid id"}` |
| Run does not exist | `404` | `{"error":"run not found"}` |
| DB / replay failure | `500` | `{"error":"failed to compute analytics"}` |

Handler skeleton (lives in `main.go`, mirrors `handleGetRun`):

```go
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
```

`computeAnalytics(ctx, run)` is an unexported method on `*server` that calls the
new Store methods (§2), runs the candidate replay (§3.6) using `s.lists`, computes
Wilson intervals (§3.1), and assembles `AnalyticsResponse`. Keep the heavy lifting
in `computeAnalytics` so it is unit-testable with a MockStore.

### 1.3 Empty / partial-run behavior (MUST handle)

- **Run with zero LLM games** (e.g. only baselines, or not started): every array
  field is an empty JSON array `[]` (never `null`). `meta.totalLlmGames = 0`.
- **Run in progress:** only completed generations have games; return whatever
  rows exist. Do not error.
- **Generation with zero games:** omit it from per-gen arrays (rows are produced
  by `GROUP BY`, so absent groups simply do not appear). The frontend tolerates
  gaps in `genIndex`.

### 1.4 Response shape (`AnalyticsResponse`)

Concrete wire-format example for a 2-generation run, `maxGuesses = 4`,
10 LLM games per generation:

```json
{
  "runId": 7,
  "maxGuesses": 4,
  "meta": {
    "totalLlmGames": 20,
    "generations": 2
  },
  "solveRateCI": [
    { "genIndex": 0, "n": 10, "wins": 6, "solveRate": 0.6, "ciLower": 0.313, "ciUpper": 0.832 },
    { "genIndex": 1, "n": 10, "wins": 8, "solveRate": 0.8, "ciLower": 0.490, "ciUpper": 0.943 }
  ],
  "winDistribution": [
    { "genIndex": 0, "total": 10, "wonByTurn": { "1": 0, "2": 1, "3": 2, "4": 3 }, "lost": 4 },
    { "genIndex": 1, "total": 10, "wonByTurn": { "1": 0, "2": 2, "3": 4, "4": 2 }, "lost": 2 }
  ],
  "turnInfoGain": [
    { "genIndex": 0, "turnIndex": 0, "meanInfoGain": 4.12, "n": 10 },
    { "genIndex": 0, "turnIndex": 1, "meanInfoGain": 2.87, "n": 10 },
    { "genIndex": 0, "turnIndex": 2, "meanInfoGain": 1.55, "n": 8 },
    { "genIndex": 0, "turnIndex": 3, "meanInfoGain": 0.74, "n": 6 },
    { "genIndex": 1, "turnIndex": 0, "meanInfoGain": 4.30, "n": 10 }
  ],
  "openingWords": [
    {
      "genIndex": 0,
      "words": [
        { "word": "crane", "count": 6 },
        { "word": "slate", "count": 3 },
        { "word": "adieu", "count": 1 }
      ]
    },
    {
      "genIndex": 1,
      "words": [
        { "word": "slate", "count": 9 },
        { "word": "crane", "count": 1 }
      ]
    }
  ],
  "remainingCandidates": [
    { "gameId": 41, "genIndex": 0, "answer": "jumbo", "remainingCandidates": 3, "numGuesses": 4 },
    { "gameId": 47, "genIndex": 0, "answer": "fizzy", "remainingCandidates": 7, "numGuesses": 4 },
    { "gameId": 58, "genIndex": 1, "answer": "vivid", "remainingCandidates": 2, "numGuesses": 4 }
  ],
  "tokenEfficiency": [
    { "genIndex": 0, "playerTokens": 18240, "reflectorTokens": 3110, "tokensUsed": 21350, "split": true },
    { "genIndex": 1, "playerTokens": 16880, "reflectorTokens": 0, "tokensUsed": 16880, "split": true }
  ],
  "reasoningVerbosity": [
    { "gameId": 40, "genIndex": 0, "won": true,  "reasoningChars": 820,  "numGuesses": 3 },
    { "gameId": 41, "genIndex": 0, "won": false, "reasoningChars": 1540, "numGuesses": 4 },
    { "gameId": 52, "genIndex": 1, "won": true,  "reasoningChars": 610,  "numGuesses": 2 }
  ],
  "wordDifficulty": [
    { "answer": "fizzy", "games": 2, "wins": 0, "winRate": 0.0 },
    { "answer": "jumbo", "games": 2, "wins": 1, "winRate": 0.5 },
    { "answer": "slate", "games": 2, "wins": 2, "winRate": 1.0 }
  ]
}
```

Field-level notes:

- `wonByTurn` keys are stringified turn numbers `"1"`..`"maxGuesses"`. Every turn
  in `[1, maxGuesses]` is present (zero-filled) so the frontend can build a stable
  stacked-bar series without probing for missing keys.
- `tokenEfficiency[].split` is `false` for legacy rows where both token columns are
  `0` but `tokensUsed > 0` (frontend then shows the combined bar only).
- `solveRate` is `wins / n`; when `n = 0` the row is omitted entirely.
- All monetary-like counts are integers; all rates/bits are JSON numbers rounded
  server-side to 3 decimals (rates) / 2 decimals (bits) as shown.

### 1.5 Go response structs (`cmd/server/main.go`)

```go
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
```

> `map[int]int` marshals to JSON with string keys (`{"1":0,...}`) — this is the
> documented Go `encoding/json` behavior for integer-keyed maps and matches the
> wire example.

---

## 2. Store Interface Additions

Add four methods to the `Store` interface in `internal/store/store.go`. They return
plain aggregate rows; all statistics (Wilson, bucketing) are computed in the
handler from these rows. Replay (#5) reuses existing `ListGames` + `ListGuesses`.

```go
// analytics (all filter agent_type = 'llm')
GameOutcomeCounts(ctx context.Context, runID int64) ([]OutcomeCount, error)
TurnInfoGainStats(ctx context.Context, runID int64) ([]TurnInfoGainStat, error)
OpeningWordCounts(ctx context.Context, runID int64) ([]OpeningWordCount, error)
ReasoningVerbosityStats(ctx context.Context, runID int64) ([]ReasoningStat, error)
WordDifficultyStats(ctx context.Context, runID int64) ([]WordDifficultyStat, error)
```

That is **five** methods (outcome counts feed both metric #1 and #2). Supporting
row types (in `store.go`):

```go
type OutcomeCount struct {
    GenIndex   int
    Won        bool
    NumGuesses int
    Count      int
}

type TurnInfoGainStat struct {
    GenIndex     int
    TurnIndex    int
    MeanInfoGain float64
    N            int
}

type OpeningWordCount struct {
    GenIndex int
    Guess    string
    Count    int
}

type ReasoningStat struct {
    GameID         int64
    GenIndex       int
    Won            bool
    ReasoningChars int
    NumGuesses     int
}

type WordDifficultyStat struct {
    Answer string
    Games  int
    Wins   int
}
```

### 2.1 SQLite implementations (`internal/store/store_sqlite.go`)

All queries are read-only `QueryContext` loops following the existing scan
pattern (return empty slice, never nil; check `rows.Err()`).

**`GameOutcomeCounts`** — feeds metrics #1 (Wilson CI) and #2 (win distribution):

```sql
SELECT gen_index, won, num_guesses, COUNT(*) AS cnt
FROM games
WHERE run_id = ? AND agent_type = 'llm'
GROUP BY gen_index, won, num_guesses
ORDER BY gen_index ASC, won ASC, num_guesses ASC;
```

Scan `won` as `int` then `Won = won != 0` (matches existing `ListGames`).

**`TurnInfoGainStats`** — metric #3:

```sql
SELECT g.gen_index, gu.turn_index,
       AVG(gu.info_gain_bits) AS mean_ig,
       COUNT(*) AS n
FROM guesses gu
JOIN games g ON gu.game_id = g.id
WHERE g.run_id = ? AND g.agent_type = 'llm'
GROUP BY g.gen_index, gu.turn_index
ORDER BY g.gen_index ASC, gu.turn_index ASC;
```

**`OpeningWordCounts`** — metric #4 (first guess only):

```sql
SELECT g.gen_index, gu.guess, COUNT(*) AS cnt
FROM guesses gu
JOIN games g ON gu.game_id = g.id
WHERE g.run_id = ? AND g.agent_type = 'llm' AND gu.turn_index = 0
GROUP BY g.gen_index, gu.guess
ORDER BY g.gen_index ASC, cnt DESC, gu.guess ASC;
```

**`ReasoningVerbosityStats`** — metric #8 (per-game reasoning length):

```sql
SELECT g.id, g.gen_index, g.won, g.num_guesses,
       COALESCE(SUM(LENGTH(gu.reasoning_text)), 0) AS chars
FROM games g
LEFT JOIN guesses gu ON gu.game_id = g.id
WHERE g.run_id = ? AND g.agent_type = 'llm'
GROUP BY g.id
ORDER BY g.gen_index ASC, g.id ASC;
```

`LEFT JOIN` so a game with no stored reasoning still yields a row with
`chars = 0`. SQLite `LENGTH()` counts characters for TEXT (NULL rows excluded by
`SUM`/`COALESCE`).

**`WordDifficultyStats`** — metric #9 (per-answer win rate):

```sql
SELECT answer, COUNT(*) AS games, SUM(won) AS wins
FROM games
WHERE run_id = ? AND agent_type = 'llm'
GROUP BY answer
ORDER BY (CAST(SUM(won) AS REAL) / COUNT(*)) ASC, answer ASC;
```

`winRate = wins / games` is computed in Go (avoids SQLite REAL rounding drift in
the wire payload). Hardest words sort first.

### 2.2 MockStore (`internal/store/mock.go`)

`MockStore` is a **real thread-safe in-memory implementation** (maps for
`runs`/`generations`/`games`/`guesses`) with per-method error-injection fields
(`Err*`). Match this existing convention — do **not** introduce function-field
stubs. Implement the five new methods to compute the aggregations over the
in-memory maps, exactly mirroring the SQLite semantics (filter
`agent_type == "llm"`; group/average in Go), and add one error-injection field
each:

```go
// add to the MockStore struct, alongside the existing Err* fields
ErrGameOutcomeCounts       error
ErrTurnInfoGainStats       error
ErrOpeningWordCounts       error
ErrReasoningVerbosityStats error
ErrWordDifficultyStats     error
```

Each method: if its `Err*` field is non-nil, return it; otherwise iterate the
in-memory `games[runID]` (and `guesses[gameID]` where the query joins guesses),
apply the `agent_type == "llm"` filter, and return the aggregated rows sorted to
match the SQL `ORDER BY` so handler tests are deterministic. This keeps MockStore
a faithful stand-in (the same fixtures used for `ListGames`/`ListGuesses` tests
drive the analytics tests) and preserves the error-injection testing pattern.

---

## 3. Per-Metric Computation Spec & Edge Cases

### 3.1 Metric 1 — Solve Rate with 95% Wilson Confidence Interval

**Source:** `GameOutcomeCounts`. Aggregate per `genIndex`: `n = Σ count`,
`wins = Σ count where won`.

**Wilson score interval** (z = 1.96 for 95%), computed in a pure helper
`wilson(wins, n int) (lower, upper float64)`:

```
p  = wins / n
z  = 1.96
z2 = z*z
denom  = 1 + z2/n
center = (p + z2/(2n)) / denom
margin = (z / denom) * sqrt( p*(1-p)/n + z2/(4*n*n) )
lower  = clamp01(center - margin)
upper  = clamp01(center + margin)
```

**Why Wilson, not normal-approximation (Wald):** Wald intervals are degenerate at
the boundaries (a 10/10 solve rate gives a zero-width Wald interval) and
undercover for small n — exactly the regime here (`gamesPerGen` is often 10–50).
Wilson stays inside [0,1] and behaves correctly at p = 0 and p = 1.

**Edge cases (MUST):**
- `n = 0` → omit the generation from `solveRateCI` (no games, interval undefined).
- `n = 1` → valid; interval is wide. Do not special-case.
- `wins = 0` → `p = 0`, `lower = 0`, `upper > 0`. Verified: with n=10, wins=0 →
  upper ≈ 0.278.
- `wins = n` → `p = 1`, `upper = 1`, `lower < 1`. With n=10 → lower ≈ 0.722.
- Always `clamp01` both bounds to guard floating error at the extremes.

**Reference value (used in the §1.4 example):** n=10, wins=6 →
solveRate=0.6, ciLower≈0.313, ciUpper≈0.832.

### 3.2 Metric 2 — Win Distribution by Turn (stacked bar per generation)

**Source:** `GameOutcomeCounts`. For each `genIndex`, initialize
`wonByTurn[t] = 0` for `t in [1, maxGuesses]`. For each row: if `won`, increment
`wonByTurn[numGuesses]`; else add `count` to `lost`. `total = Σ count`.

**Edge cases:**
- A won game always has `1 <= numGuesses <= maxGuesses`. Defensive: if a
  malformed row has `numGuesses` outside `[1, maxGuesses]`, clamp into range and
  do not drop it (counts must sum to `total`).
- Lost games have `numGuesses == maxGuesses` (loss = exhausted turns) but are
  always counted under `lost`, never under a turn bucket.
- Generation with all losses → all `wonByTurn` zero, `lost == total`.

### 3.3 Metric 3 — Turn-Level Information Gain (grouped bar: turn × generation)

**Source:** `TurnInfoGainStats` directly → `TurnInfoGainPoint`. Round
`meanInfoGain` to 2 decimals.

**Edge cases:**
- Later turns have fewer samples (`n` shrinks as games end early). `n` is returned
  per point so the frontend can de-emphasize low-sample turns (e.g. tooltip "n=6").
- `info_gain_bits` is `0` when a guess eliminated nothing or `before == 0`
  (see `wordle.InfoGainBits`); these legitimately pull the mean down — do not
  filter them.
- No guesses for a (gen, turn) pair → that point is simply absent.

### 3.4 Metric 4 — Opening Word Frequency (bar chart per generation)

**Source:** `OpeningWordCounts` grouped into `OpeningWordsPoint` per `genIndex`,
`words` already sorted by count desc (SQL `ORDER BY cnt DESC, guess ASC`).

**Edge cases:**
- The opening guess is `turn_index = 0`. Every LLM game has one, including games
  that immediately violated constraints (the fallback word is still recorded as
  the turn-0 guess) — this is intended; opening-word concentration includes
  fallbacks.
- A generation may legitimately have many distinct openers (high temperature) →
  `words` can be long. Frontend caps the displayed bars to the top 12 and labels
  the remainder "+N more" (display-only; payload returns all).

### 3.5 Metric 5 — Remaining Candidates at Game-Over (losses only)

**Source:** Go replay in `computeAnalytics`, no new SQL beyond existing methods.

Algorithm:
1. `games, _ := s.store.ListGames(ctx, runID, nil)`.
2. For each game with `agent_type == "llm" && !won`:
   a. `guesses, _ := s.store.ListGuesses(ctx, game.ID)` (ordered by turn).
   b. `candidates := copy(s.lists.Answers)`.
   c. For each guess in turn order:
      `fb := wordle.FromString(guess.Feedback)` then
      `candidates = wordle.FilterCandidates(candidates, guess.Guess, fb)`.
   d. `remainingCandidates = len(candidates)` after the final guess.
3. Append `RemainingCandPoint{gameId, genIndex, answer, remainingCandidates, numGuesses}`.

This reuses the exact same scoring/filtering functions the live engine used
(`wordle.ScoreGuess` via `FromString`+`FilterCandidates`), guaranteeing the replay
is consistent with how `info_gain_bits` was originally computed.

**Edge cases (MUST):**
- **Won games are excluded** — by definition a won game ends with 1 remaining
  candidate (the answer); the research question is "how close was the agent when
  it *lost*."
- **Game with zero guesses** (degenerate) → `remainingCandidates = len(Answers)`
  (full list, nothing filtered). Still emit the row.
- **Answer not in the answer list** (shouldn't happen; answers are sampled from
  the list) → replay still works; `remainingCandidates` may reach 0 if guesses
  over-constrain. If `0`, return `0` (do not clamp to 1; this is observation, not
  info-gain math).
- **Feedback string malformed / not length 5** → `wordle.FromString` returns the
  zero Feedback (all-gray), which `FilterCandidates` applies literally. Acceptable
  for a defensive replay; such rows are not expected from the engine.
- **No lost LLM games** → empty array.

> Performance: O(lostGames × turns × |Answers|). For a worst-case run
> (50 gens × 500 games, all losses, |Answers|≈2,300) this is bounded and runs in
> well under a second; the single-connection SQLite pool serializes the
> `ListGuesses` calls but volume is small. No new index required (existing
> `idx_guesses_game` covers the per-game lookups; `idx_games_run` covers the list).

### 3.6 Metric 6 — Prompt Edit Distance (client-side, no endpoint)

Computed entirely in the frontend from `run.generationsData[i].promptText`.

**Normalized Levenshtein** between consecutive generations:

```
norm(a, b) = levenshtein(a, b) / max(len(a), len(b))
```

Series point for generation `i (>= 1)`: `{ genIndex: i, distance: norm(prompt[i-1], prompt[i]) }`.

**Edge cases (MUST):**
- Generation 0 has no predecessor → it has **no point** (series starts at gen 1).
  Document this so the x-axis is not mistaken for off-by-one.
- `prompt[i-1] == prompt[i]` → distance `0.0` (reflector reused the prompt, e.g.
  unparseable reflector output, or the final generation which never reflects).
- Both prompts empty (`max == 0`) → define distance `0.0` (avoid divide-by-zero).
- Distance is always in `[0, 1]`. Render as percentage on the y-axis.
- Use rune length (`[...prompt].length`) not UTF-16 `.length`, and operate on code
  points in the Levenshtein DP, so multi-byte characters are counted as one edit
  (consistent with the backend's rune-based `prompt_len`).

Place the implementation in `frontend/src/lib/analytics.ts` (`levenshtein`,
`normalizedLevenshtein`).

### 3.7 Metric 7 — Token Efficiency (player vs reflector per generation)

Requires the **token split** (§4). `tokenEfficiency` is built from the
`generations` rows (already loaded via `ListGenerations` in `computeAnalytics`):

```
playerTokens    = generation.PlayerTokens
reflectorTokens = generation.ReflectorTokens
tokensUsed      = generation.TokensUsed
split           = (playerTokens + reflectorTokens) > 0
```

**Edge cases:**
- **Final generation** never reflects → `reflectorTokens == 0` legitimately
  (see §1.4 gen 1). This is correct, not missing data.
- **Legacy rows** (pre-migration): `player = reflector = 0`, `tokensUsed > 0` →
  `split = false`. Frontend renders a single combined bar and a footnote
  "token split unavailable for this run."
- **LLM never used** (fallback path): all tokens `0`, `split = false`.

### 3.8 Metric 8 — Reasoning Verbosity vs Outcome (scatter)

**Source:** `ReasoningVerbosityStats` → `ReasoningPoint` directly. One point per
LLM game: x = `reasoningChars`, y/color = `won`, series/color also by `genIndex`.

**Edge cases:**
- Games with no reasoning stored → `reasoningChars = 0` (still plotted on the
  y-baseline; meaningful — terse runs cluster near 0).
- Baseline games excluded (no reasoning, agent_type filter).
- Large runs → many points; frontend uses small dot radius and opacity; no
  server-side sampling (payload is one small row per game).

### 3.9 Metric 9 — Per-Word Difficulty (horizontal bar, sorted by win rate)

**Source:** `WordDifficultyStats`. `winRate = wins / games` computed in Go,
rounded to 3 decimals. Rows already sorted hardest-first by SQL.

**Edge cases:**
- Each answer typically appears once per generation (same seeded sample reused
  every generation), so `games` ≈ number of generations the word was played in.
  This is intended: a word the agent fails across generations is a persistent
  blind spot.
- `games` can be 1 → `winRate ∈ {0, 1}`; frontend should show `games` (n) in the
  tooltip so single-sample words are not over-interpreted.
- Ties in win rate broken by `answer` ascending (stable, deterministic).

---

## 4. Token Split — Schema, Store, and `experiment.go`

### 4.1 Migration (`internal/store/store_sqlite.go` → `Migrate`)

Append two idempotent `ALTER TABLE` statements after the existing `max_guesses`
ALTER (same swallow-error pattern):

```go
_, _ = s.db.ExecContext(ctx, `ALTER TABLE generations ADD COLUMN player_tokens INTEGER NOT NULL DEFAULT 0`)
_, _ = s.db.ExecContext(ctx, `ALTER TABLE generations ADD COLUMN reflector_tokens INTEGER NOT NULL DEFAULT 0`)
```

Also add the columns to `migrations/001_initial.up.sql` (for fresh databases) so
the canonical schema and the ALTER path agree:

```sql
    player_tokens INTEGER NOT NULL DEFAULT 0,
    reflector_tokens INTEGER NOT NULL DEFAULT 0,
```

(Place them after `tokens_used` in the `generations` table definition.)

### 4.2 `store.Generation` struct (`internal/store/store.go`)

Add two fields (keep `TokensUsed` for backward compatibility and as the combined
total):

```go
PlayerTokens    int `json:"playerTokens"`
ReflectorTokens int `json:"reflectorTokens"`
```

### 4.3 SQLite read/write (`store_sqlite.go`)

- **`UpdateGenerationStats`** — extend the `UPDATE` to set
  `player_tokens = ?, reflector_tokens = ?` (bind `g.PlayerTokens`,
  `g.ReflectorTokens`). `tokens_used` continues to be set to the combined total.
- **`ListGenerations`** — add `player_tokens, reflector_tokens` to the `SELECT`
  and to the `rows.Scan` targets.
- **`CreateGeneration`** — no change needed (tokens are written at update time;
  defaults are 0).

### 4.4 `experiment.go` changes (`runExperiment`)

Today the generation loop accumulates a single `totalTokens` that mixes player
game tokens and the reflector call. Split it:

1. Replace `totalTokens := 0` with:
   ```go
   playerTokens := 0
   reflectorTokens := 0
   ```
2. In the game loop, `playerTokens += gr.tokensUsed` (was `totalTokens += gr.tokensUsed`).
3. After `refl.Reflect(...)`:
   ```go
   reflectorTokens += refUsage.InputTokens + refUsage.OutputTokens
   ```
   (was added into `totalTokens`).
4. When persisting generation stats:
   ```go
   gen.PlayerTokens = playerTokens
   gen.ReflectorTokens = reflectorTokens
   gen.TokensUsed = playerTokens + reflectorTokens
   ```
5. The `gen_end` SSE event's `TokensUsed` field keeps reporting the combined total
   (`playerTokens + reflectorTokens`) — no SSE schema change. (Optional future
   enhancement: add `playerTokens`/`reflectorTokens` to the SSE `Event`; **out of
   scope** for this feature.)

**Why player vs reflector matters scientifically:** the reflector is the
*evolutionary operator* and the player is the *fitness evaluation*. Separating
their token cost lets a researcher quantify the overhead of self-reflection
relative to raw play, and detect prompt bloat that inflates player cost over
generations.

**Edge cases:** baseline games call no LLM (`playBaselineGame` tracks no tokens) →
they contribute nothing, correct. Final generation does not reflect →
`reflectorTokens == 0`.

---

## 5. Frontend — Component Tree

### 5.1 New files

```
frontend/src/
├── api/
│   └── types.ts                         (MODIFY — add Analytics interfaces, §5.2)
│   └── client.ts                        (MODIFY — add api.getAnalytics, §5.3)
├── lib/
│   └── analytics.ts                     (NEW — levenshtein, boxStats, pivot helpers)
├── components/
│   ├── InfoPopup.tsx                    (NEW — reusable ⓘ modal, §6)
│   ├── ChartCard.tsx                    (NEW — title + InfoPopup trigger wrapper)
│   └── analytics/
│       ├── AnalysisTab.tsx              (NEW — container: fetch + layout grid)
│       ├── SolveRateCIChart.tsx         (#1 — line + shaded CI band)
│       ├── WinDistributionChart.tsx     (#2 — stacked bar)
│       ├── TurnInfoGainChart.tsx        (#3 — grouped bar)
│       ├── OpeningWordsChart.tsx        (#4 — bar, per-gen selector)
│       ├── RemainingCandidatesChart.tsx (#5 — box plot, losses)
│       ├── PromptEditDistanceChart.tsx  (#6 — line, client-computed)
│       ├── TokenEfficiencyChart.tsx     (#7 — grouped/stacked bar)
│       ├── ReasoningScatterChart.tsx    (#8 — scatter)
│       └── WordDifficultyChart.tsx      (#9 — horizontal bar)
└── views/
    └── RunDetail.tsx                    (MODIFY — add 'analysis' tab, §7)
```

### 5.2 TypeScript interfaces (`api/types.ts`)

```ts
export interface SolveRateCIPoint {
  genIndex: number
  n: number
  wins: number
  solveRate: number
  ciLower: number
  ciUpper: number
}
export interface WinDistPoint {
  genIndex: number
  total: number
  wonByTurn: Record<string, number> // keys "1".."maxGuesses"
  lost: number
}
export interface TurnInfoGainPoint {
  genIndex: number
  turnIndex: number
  meanInfoGain: number
  n: number
}
export interface OpeningWordRow { word: string; count: number }
export interface OpeningWordsPoint { genIndex: number; words: OpeningWordRow[] }
export interface RemainingCandPoint {
  gameId: number
  genIndex: number
  answer: string
  remainingCandidates: number
  numGuesses: number
}
export interface TokenEfficiencyPoint {
  genIndex: number
  playerTokens: number
  reflectorTokens: number
  tokensUsed: number
  split: boolean
}
export interface ReasoningPoint {
  gameId: number
  genIndex: number
  won: boolean
  reasoningChars: number
  numGuesses: number
}
export interface WordDifficultyPoint {
  answer: string
  games: number
  wins: number
  winRate: number
}
export interface AnalyticsResponse {
  runId: number
  maxGuesses: number
  meta: { totalLlmGames: number; generations: number }
  solveRateCI: SolveRateCIPoint[]
  winDistribution: WinDistPoint[]
  turnInfoGain: TurnInfoGainPoint[]
  openingWords: OpeningWordsPoint[]
  remainingCandidates: RemainingCandPoint[]
  tokenEfficiency: TokenEfficiencyPoint[]
  reasoningVerbosity: ReasoningPoint[]
  wordDifficulty: WordDifficultyPoint[]
}
```

Also extend `Generation` (used by metric #7 fallback and #6):

```ts
export interface Generation {
  // ...existing fields...
  playerTokens?: number
  reflectorTokens?: number
}
```

### 5.3 API client (`api/client.ts`)

```ts
getAnalytics(id: number): Promise<AnalyticsResponse> {
  return request(`/runs/${id}/analytics`)
},
```

### 5.4 `AnalysisTab.tsx` container behavior

- Props: `{ runId: number; generations: Generation[]; maxGuesses: number }`
  (`generations` passed down so #6 needs no fetch).
- On mount (and when `runId` changes): `api.getAnalytics(runId)`, manage
  `loading` / `error` / `data` exactly like `RunDetail` does for the run fetch
  (spinner, error-box). Reuse the existing CSS classes.
- Lazy-load: only fetch when the Analysis tab is first activated (the container is
  only mounted when `tab === 'analysis'`, see §7) — avoids the extra round-trip
  for users who never open it.
- Layout: a vertical stack of `ChartCard`s (reuse the existing
  `display:flex; flex-direction:column; gap:32` pattern from `MetricsChart`). Each
  card is full width; charts use `ResponsiveContainer`.
- Empty state: if `meta.totalLlmGames === 0`, render the existing `empty-state`
  block ("No LLM games to analyze yet.") instead of the cards.

### 5.5 `ChartCard.tsx`

```ts
interface ChartCardProps {
  title: string
  metricKey: MetricKey   // selects InfoPopup content, §6.2
  children: React.ReactNode
  subtitle?: string
}
```

Renders a `card` with a header row: `title` (left) and a small circular `ⓘ`
button (right, `aria-label="About this metric"`). Clicking opens `InfoPopup` with
the content for `metricKey`. The chart (`children`) renders below the header.

### 5.6 Charting notes per metric

- **#1 SolveRateCIChart** — Recharts `ComposedChart`. Render the CI band as an
  `<Area>` of `[ciLower*100, ciUpper*100]` (use a range area: dataKey returning a
  `[low, high]` tuple, fill with low-opacity accent) **behind** a `<Line>` of
  `solveRate*100`. Reuse the green `#6aaa64` accent. Keep the existing
  random-baseline `ReferenceLine y={17}`.
- **#2 WinDistributionChart** — `BarChart` stacked. Build series keys
  `turn1..turn{maxGuesses}` plus `lost` by pivoting `wonByTurn`. Use a sequential
  color ramp for turns (fast=green → slow=amber) and a muted gray/red for `lost`.
- **#3 TurnInfoGainChart** — grouped `BarChart`; x = turn index, one bar series per
  generation (or x = generation, grouped by turn — pick x = turn, series = gen for
  ≤8 gens; if many gens, switch to a small-multiples / gen selector). Tooltip shows
  `n`.
- **#4 OpeningWordsChart** — `BarChart` with a generation `<select>` (default:
  latest gen). Show top 12 words; horizontal bars read best for word labels.
- **#5 RemainingCandidatesChart** — Recharts has **no native box plot**. Compute
  `{min,q1,median,q3,max}` per generation client-side (`boxStats` in
  `lib/analytics.ts`, linear-interpolation quartiles) and render with a
  `ComposedChart`: a floating `<Bar>` for the IQR (value `[q1,q3]` via a custom
  `shape`), a `<Line>`/reference segment for the median, and `<ErrorBar>` (or thin
  bars) for the min–max whiskers. Overlay individual loss points as a faint
  `<Scatter>` (jittered x) so small samples are honest. Document in the InfoPopup
  that boxes summarize **lost games only**.
- **#6 PromptEditDistanceChart** — `LineChart` from client-computed series
  (§3.6); y-axis 0–100%. Series starts at Gen 1.
- **#7 TokenEfficiencyChart** — grouped `BarChart`: `playerTokens` and
  `reflectorTokens` per generation. If any point has `split === false`, render a
  single `tokensUsed` bar series instead and show the "split unavailable" footnote.
- **#8 ReasoningScatterChart** — `ScatterChart`; x = `reasoningChars`, y =
  jittered `won ? 1 : 0` (two bands) or use color = won and y = genIndex. Color by
  generation with a legend; won = filled, lost = hollow.
- **#9 WordDifficultyChart** — horizontal `BarChart`, words on the y-axis sorted
  hardest-first; bar = `winRate`. Tooltip shows `wins/games`.

---

## 6. InfoPopup Component & Scientific Copy

### 6.1 Component interface (`components/InfoPopup.tsx`)

```ts
export type MetricKey =
  | 'solveRateCI'
  | 'winDistribution'
  | 'turnInfoGain'
  | 'openingWords'
  | 'remainingCandidates'
  | 'promptEditDistance'
  | 'tokenEfficiency'
  | 'reasoningVerbosity'
  | 'wordDifficulty'

export interface InfoContent {
  title: string
  whatItMeasures: string
  whyItMatters: string
  goodBad: string     // good/bad value ranges, ML-scientific framing
}

interface InfoPopupProps {
  metricKey: MetricKey
  open: boolean
  onClose: () => void
}
```

Behavior:
- Renders a centered modal overlay (reuse the existing modal pattern — the
  `GameModal` component defined inside `components/GameList.tsx` — same backdrop +
  `card` styling). Sections: **title** (h3),
  **What it measures**, **Why it matters for prompt evolution**, **How to read
  good vs. bad values**, and a **Close** button (bottom-right).
- Close on: Close button, backdrop click, and `Escape` key.
- Accessibility: `role="dialog"`, `aria-modal="true"`, `aria-labelledby` →
  the title element id; focus moves to the dialog on open and returns to the ⓘ
  trigger on close.
- Content comes from the `INFO_CONTENT: Record<MetricKey, InfoContent>` map below
  (§6.2). Copy is used **verbatim**.

### 6.2 `INFO_CONTENT` — exact copy for all 9 metrics

> Implement as a `const INFO_CONTENT: Record<MetricKey, InfoContent>` in
> `InfoPopup.tsx`. The four strings per metric map to the four `InfoContent`
> fields. Use this text verbatim.

**solveRateCI — "Solve Rate with 95% Confidence Interval"**
- *whatItMeasures:* "The fraction of games the agent solved in each generation,
  shown with a 95% Wilson score confidence interval. The shaded band is the range
  of true solve rates consistent with the observed wins, given the sample size."
- *whyItMatters:* "Prompt evolution is only meaningful if a generation's
  improvement exceeds sampling noise. With a small number of games per generation,
  a solve-rate jump can be pure chance. The confidence band tells you whether two
  generations are statistically distinguishable: if their intervals overlap
  heavily, the apparent gain may not be real. The Wilson interval is used rather
  than the normal approximation because it remains valid at the boundaries (0% and
  100%) and for small samples."
- *whyItMatters/goodBad:* "Higher is better; a tight band is better than a wide
  one. Bands shrink as games-per-generation grows (roughly with 1/√n). If you see
  an upward solve-rate trend whose later intervals sit entirely above the earlier
  ones, that is strong evidence the reflector is genuinely improving strategy. If
  the bands overlap across all generations, treat the run as inconclusive and
  increase the sample size."

**winDistribution — "Win Distribution by Turn"**
- *whatItMeasures:* "For each generation, how the solved games are distributed
  across the turn on which they were won (turn 1, 2, …), plus the count of lost
  games. Each bar is one generation; segments stack to the total games played."
- *whyItMatters:* "Solve rate alone hides *how* the agent wins. Two prompts with
  the same solve rate can differ sharply in efficiency: one may grind out wins on
  the final allowed turn while another wins early with confident information-dense
  guesses. Watching mass shift toward earlier turns across generations is direct
  evidence that the reflector is teaching the agent to gather information faster,
  not just to avoid losing."
- *goodBad:* "Healthy evolution shifts the colored mass leftward (earlier-turn
  wins) and shrinks the 'lost' segment over generations. A distribution piled up
  on the last allowed turn indicates a fragile strategy that barely succeeds and
  will collapse under a tighter guess budget. A growing 'lost' segment is a
  regression signal."

**turnInfoGain — "Turn-Level Information Gain"**
- *whatItMeasures:* "The mean information gain, in bits, contributed by each guess
  position (turn 1, turn 2, …) within a game, broken down by generation. One bit
  means the guess halved the set of remaining candidate answers."
- *whyItMatters:* "Wordle is, formally, an active-learning problem: each guess is
  an experiment that should maximally reduce answer uncertainty. This chart shows
  whether the evolved prompt front-loads information — strong openers that
  eliminate large candidate sets — and how gain decays over the course of a game.
  It separates *opening strategy* from *endgame strategy*, which solve rate cannot."
- *goodBad:* "Early turns should show the highest bits (a good opener on the full
  answer list yields roughly 4–6 bits). Bits naturally decay on later turns as
  fewer candidates remain — that decay is expected, not a problem. A weak or flat
  turn-1 bar across generations means the reflector has not discovered
  high-entropy openings. Note the per-point sample size (n) in the tooltip: late
  turns are estimated from fewer games and are noisier."

**openingWords — "Opening Word Frequency"**
- *whatItMeasures:* "The distribution of first guesses the agent chooses in each
  generation, and how often each is used."
- *whyItMatters:* "The opening word is the single highest-leverage decision in
  Wordle and a clean fingerprint of strategy. Convergence onto a small set of
  high-entropy openers (e.g. words rich in common letters) across generations is a
  visible sign that prompt evolution is discovering and committing to a strong
  fixed policy. Persistent scatter across many openers indicates the prompt has
  not constrained the opening, leaving it to model temperature and chance."
- *goodBad:* "Increasing concentration on one or a few strong openers over
  generations is the desirable trend — it shows the reflector is encoding a
  reusable heuristic. High diversity that never narrows suggests an
  under-specified prompt or excessive temperature. Beware concentration on a
  *weak* opener: cross-reference with Turn-Level Information Gain to confirm the
  favored opener actually yields high bits."

**remainingCandidates — "Remaining Candidates at Game-Over (losses)"**
- *whatItMeasures:* "For every *lost* game, the number of answer candidates still
  consistent with all feedback when the guess budget ran out, computed by
  replaying the agent's guesses against the answer word list. Shown as a box plot
  per generation (median, interquartile range, and range), losses only."
- *whyItMatters:* "This diagnoses *why* the agent loses. A small number of
  remaining candidates at game-over means the agent had nearly solved the puzzle
  and lost on the last step — a near-miss, often a guess-budget or tie-breaking
  problem. A large number means the agent failed to constrain the search at all —
  a fundamental strategy failure. The two cases call for completely different
  prompt fixes, and solve rate alone cannot tell them apart."
- *goodBad:* "Lower is better: boxes near 1–3 remaining candidates indicate
  'unlucky near-misses' that a small guess-budget or end-game tweak could convert
  to wins. Boxes in the tens or hundreds indicate the agent is not exploiting
  feedback — the prompt needs stronger constraint-tracking and elimination
  guidance. A downward shift of the boxes across generations means the reflector
  is teaching the agent to box-in the answer even on its failures."

**promptEditDistance — "Prompt Edit Distance"**
- *whatItMeasures:* "The normalized Levenshtein (character edit) distance between
  each generation's strategy prompt and the previous generation's, expressed as a
  fraction of the longer prompt (0% = identical, 100% = completely rewritten)."
- *whyItMatters:* "This quantifies the *magnitude of mutation* the reflector
  applies at each step — the size of the evolutionary jump. The reflector is
  instructed to make surgical, targeted edits; this metric verifies it. Large
  oscillating edits suggest the reflector is thrashing (rewriting wholesale rather
  than refining), while distances trending toward zero suggest the search has
  converged on a stable prompt."
- *goodBad:* "Small, decreasing edits (a few percent, shrinking over generations)
  indicate healthy convergence toward a stable strategy. Persistently large edits
  (tens of percent) indicate instability or prompt thrashing — often correlated
  with oscillating solve rate. A flat 0% means the reflector stopped changing the
  prompt (it reused the prior prompt, e.g. unparseable reflector output or the
  final, non-reflecting generation). Read this chart alongside Solve Rate: useful
  evolution shows shrinking edits *and* rising solve rate."

**tokenEfficiency — "Token Efficiency (player vs reflector)"**
- *whatItMeasures:* "Per generation, the LLM tokens consumed by the player (all
  game-play calls) versus the reflector (the single self-reflection call that
  rewrites the prompt)."
- *whyItMatters:* "Self-improvement has a compute cost, and this splits it into
  its two functional parts: the player is the fitness evaluation, the reflector is
  the evolutionary operator. Tracking them separately reveals the overhead of
  reflection relative to raw play, and exposes *prompt bloat* — if player tokens
  climb generation over generation, the evolved prompt is growing longer and more
  expensive to run at inference time, a hidden cost of evolution."
- *goodBad:* "There is no universally 'good' value — interpret trends. Flat or
  declining player tokens alongside rising solve rate is ideal (the agent gets
  better without getting more expensive). Steadily rising player tokens signal
  prompt bloat; weigh the accuracy gain against the inference cost. Reflector
  tokens are incurred once per generation and are zero for the final generation
  (which never reflects) — that zero is expected, not missing data."

**reasoningVerbosity — "Reasoning Verbosity vs Outcome"**
- *whatItMeasures:* "For each game, the total number of characters of
  chain-of-thought reasoning the agent produced across all of its guesses, plotted
  against whether the game was won or lost, colored by generation."
- *whyItMatters:* "It probes the relationship between deliberation and success. Does
  the evolved prompt make the agent reason more, and does more reasoning actually
  help? A positive association (won games cluster at higher verbosity) supports
  reasoning-eliciting prompt changes; a negative or null association suggests
  verbosity is wasted tokens — or even a symptom of the model floundering on hard
  words. It also surfaces whether reflection is inadvertently inflating
  reasoning length over generations."
- *goodBad:* "There is no single target length; look at separation. If won and
  lost games occupy clearly different verbosity ranges, reasoning length is
  informative. If the two outcomes are fully intermixed, verbosity is not
  predictive and any prompt change that merely increases it is paying tokens for
  nothing. Watch for runaway verbosity in later generations with no solve-rate
  benefit — a sign the reflector is rewarding length over substance."

**wordDifficulty — "Per-Word Difficulty"**
- *whatItMeasures:* "Each answer word's win rate aggregated across all generations,
  sorted from hardest (lowest win rate) to easiest. The tooltip shows the number
  of games behind each rate."
- *whyItMatters:* "It identifies the agent's systematic blind spots — words it
  fails on repeatedly regardless of prompt generation. Persistent failures often
  share structure (rare letters, repeated letters, many near-anagrams) and point
  to concrete, targetable weaknesses the reflector should address. It also
  separates *strategy* problems from *vocabulary* problems: a word missed every
  time across many generations is unlikely to be fixed by yet another strategy
  tweak."
- *goodBad:* "A short, shrinking tail of hard words is good — it means failures are
  rare and idiosyncratic. A long flat tail of zero-win-rate words is a red flag:
  the agent has structural blind spots that prompt evolution is not resolving.
  Treat words with very few games (n = 1) cautiously — a single loss reads as 0%
  win rate but is statistically weak; rely on words with more games when
  diagnosing persistent weaknesses."

---

## 7. Tab Integration (`views/RunDetail.tsx`)

1. Extend the `Tab` union:
   ```ts
   type Tab = 'charts' | 'prompts' | 'games' | 'analysis'
   ```
2. Add `'analysis'` to the tab button list and its label:
   ```ts
   {(['charts', 'prompts', 'games', 'analysis'] as Tab[]).map((t) => ( ... ))}
   // label: t === 'analysis' ? 'Analysis' : ...
   ```
3. Render the new tab body (mount only when active, so the analytics fetch is
   lazy):
   ```tsx
   {tab === 'analysis' && (
     gens.length === 0
       ? <div className="empty-state"><p>No generations completed yet.</p></div>
       : <AnalysisTab runId={runId} generations={gens} maxGuesses={run.maxGuesses} />
   )}
   ```
4. No change to the summary-stats header or the existing three tabs. `gens` and
   `run.maxGuesses` are already in scope in `RunDetail`.

Placement: `Analysis` sits **after** `Games` in the tab bar (Charts → Prompt
Timeline → Games → Analysis), so the existing default tab (`charts`) and muscle
memory are unchanged.

---

## 8. Implementation Order & Parallelization

### Phase 1 — unblock both tracks (do first)

- **Backend (B1):** Token-split schema migration + `store.Generation` fields +
  `UpdateGenerationStats`/`ListGenerations` SQL + `experiment.go` split (§4).
  Independent of the endpoint; ship and verify `go test ./...` green.
- **Frontend (F1):** `InfoPopup.tsx` + `INFO_CONTENT` (§6) + `ChartCard.tsx` +
  `lib/analytics.ts` (`levenshtein`, `normalizedLevenshtein`, `boxStats`). These
  need no backend and are fully testable against the copy and the math in this doc.
- **Frontend (F2, parallel):** Add the `Analysis` tab shell to `RunDetail.tsx`
  and `AnalysisTab.tsx` container with a **mocked** `AnalyticsResponse` matching
  §1.4, so charts can be built before the API exists.

### Phase 2 — backend endpoint

- **Backend (B2):** Add the five `Store` methods + SQLite queries (§2) + MockStore
  stubs.
- **Backend (B3):** `computeAnalytics` + `handleGetAnalytics` + route (§1), incl.
  Wilson helper (§3.1), win-distribution bucketing (§3.2), and candidate replay
  (§3.6). Depends on B2.

### Phase 3 — wire-up & charts

- **Frontend (F3):** `api.getAnalytics` + `types.ts` interfaces (§5.2–5.3); swap
  the mock for the real fetch.
- **Frontend (F4):** Build the 9 chart components (§5.6). #6 (edit distance) and
  the #1 CI-band enhancement can start in Phase 1/2 since #6 needs no endpoint and
  #1's shape is fixed here.

### Phase 4 — QA

- **QA (Q1, can start in Phase 1):** Table-driven tests for the pure helpers —
  `wilson()` (boundary cases p=0, p=1, n=1), win-distribution bucketing, and
  client `levenshtein`/`normalizedLevenshtein`/`boxStats` (empty, single, equal,
  unicode).
- **QA (Q2):** `handleGetAnalytics` tests with MockStore fixtures asserting the
  full `AnalyticsResponse` JSON, plus 400/404/empty-run paths (§1.2–1.3).
- **QA (Q3):** Replay correctness (§3.6) — a known lost game with hand-computed
  remaining-candidate count; verify won games are excluded and zero-guess games
  return the full list.

### Dependency summary

```
B1 ─┐
F1 ─┼─▶ (independent, Phase 1)
F2 ─┘
B2 ─▶ B3 ─▶ F3 ─▶ F4
Q1 (Phase 1) ; Q2,Q3 after B3
```

---

## 9. Quality Checklist (verify before sign-off)

- [ ] Every analytics SQL query filters `agent_type = 'llm'` (Decision A).
- [ ] `wonByTurn` is zero-filled for every turn in `[1, maxGuesses]`.
- [ ] Wilson interval clamps to `[0,1]`; `n=0` generations are omitted.
- [ ] Won games are excluded from `remainingCandidates`; zero-guess games return
      the full answer-list size.
- [ ] Token split: `tokensUsed == playerTokens + reflectorTokens` for new rows;
      legacy rows set `split=false`.
- [ ] Prompt edit-distance series starts at Gen 1 (Gen 0 has no point); uses code
      points, not UTF-16 units.
- [ ] All arrays serialize as `[]` (never `null`) for empty runs.
- [ ] MockStore implements all five new methods; `go test ./...`, `go vet ./...`,
      `golangci-lint run ./...` pass.
- [ ] `InfoPopup` copy matches §6.2 verbatim; modal closes on Escape/backdrop/Close
      and is keyboard-accessible.
- [ ] New endpoint is registered inside the authenticated route group.
```
