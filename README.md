# promptevo

**An LLM agent that plays Wordle, reflects on its own performance, and rewrites its
own strategy prompt — generation after generation.** promptevo is a research
harness for studying *in-context meta-learning*: can a language model improve at a
task purely by editing the natural-language instructions it gives itself, with no
weight updates and no human in the loop?

---

## 1. Quick Start

Run promptevo with **Docker Compose using pre-built images from Docker Hub** — no
git clone and no local build required. You only need Docker (with the Compose
plugin) and two files: a `.env` and the `docker-compose.yml`.

**1. Create a `.env` file.** If you have the repo, copy `.env.example`; otherwise
create `.env` by hand with the variables you need:

- `LLM_PROVIDER` — the active provider: `openrouter`, `anthropic`, or `openai`.
- `OPENROUTER_API_KEY` — required when using OpenRouter.
- `ANTHROPIC_API_KEY` — required when using Anthropic direct.
- `OPENAI_API_KEY` — required when using OpenAI direct.
- `AUTH_USERNAME` and `AUTH_PASSWORD` — recommended for any public deployment;
  leave blank to disable login on a local/trusted network.

Minimal example using OpenRouter (the default provider):

```bash
# .env
LLM_PROVIDER=openrouter
OPENROUTER_API_KEY=sk-or-...

# recommended for any public deployment
AUTH_USERNAME=admin
AUTH_PASSWORD=change-me
```

**2. Download only the `docker-compose.yml`** (no need to clone the repo):

```bash
curl -O https://raw.githubusercontent.com/vpoluyaktov/promptevo/main/docker-compose.yml
```

**3. Start the app.** Docker pulls the pre-built images from Docker Hub
automatically — `vpoluyaktov/promptevo-backend:latest` and
`vpoluyaktov/promptevo-frontend:latest`:

```bash
docker compose up -d
```

**4. Open** <http://localhost:3001>.

> The app boots even **without** an API key — you can browse the UI and any past
> runs. Starting a *new* run requires the active provider's API key; without it
> `POST /api/runs` returns `503 LLM gateway not configured`.

---

## 2. Research framing

**In-context meta-learning** here means self-improvement that lives entirely in the
prompt. The agent is never fine-tuned. Instead, after each *generation* of games it
is shown its own aggregate statistics (solve rate, mean guesses, rule violations,
information gain) together with a handful of failed games, and is asked to diagnose
its weaknesses and produce a *better strategy prompt*. That rewritten prompt becomes
the agent's instructions for the next generation. Learning is therefore a closed
loop of **play → reflect → rewrite → play**, and the only thing that changes between
generations is a block of text the model wrote about itself.

**Why Wordle is the right testbed.** Wordle is unusually well-suited to measuring
this kind of self-improvement cleanly:

- **Low stochasticity** — given a fixed answer, the feedback for any guess is fully
  deterministic. With a fixed seed the *same* set of answers is replayed every
  generation, so changes in performance reflect prompt quality, not word luck.
- **Discrete, finite action space** — a guess is one of a known list of valid
  five-letter words. Mistakes (reusing a gray letter, ignoring a green) are
  unambiguous and machine-checkable, which gives us a hard *violation rate* signal.
- **A known optimum** — optimal Wordle play is a solved problem (entropy-maximizing
  openers, candidate filtering). We ship deterministic non-LLM baselines (random,
  letter-frequency, entropy) so the LLM's curve can be read against a known ceiling
  and floor rather than in a vacuum.

**What the cross-model comparison reveals.** Because every run is seeded and every
metric is fixed, two different models can be run on *identical* words and scored on
the same axes. This turns "which model is better at Wordle" into the sharper
question this project actually cares about: **which model is better at improving
itself** — does it find a better strategy faster, does its prompt converge or
oscillate, does it trade violations for solve rate, and does a strong *reflector*
model lift a weaker *player* model?

---

## 3. Architecture

```
        ┌─────────────────────────────────────────────┐
        │            Browser — React SPA             │
        │   Runs list · New Run form · Run detail     │
        │   Recharts dashboards · live SSE feed       │
        └───────────────────┬─────────────────────────┘
                            │  HTTP (JSON) + SSE
                            ▼
        ┌─────────────────────────────────────────────┐
        │        frontend — nginx (port 3000)        │
        │   serves the static React bundle            │
        │   proxies /api/*  →  backend:8080           │
        │   SSE-safe (proxy_buffering off)            │
        └───────────────────┬─────────────────────────┘
                            │  http://backend:8080
                            ▼
        ┌─────────────────────────────────────────────┐
        │        backend — Go service (port 8080)    │
        │   chi router → REST + SSE handlers          │
        │   experiment orchestrator (per-run goroutine)│
        │   agent · reflector · wordle · baselines    │
        │   llm.Client ──HTTPS──► OpenRouter/Anthropic/OpenAI │
        │   store ──► modernc.org/sqlite (CGO-free)   │
        └───────────────────┬─────────────────────────┘
                            ▼
                  ┌───────────────────┐
                  │  Docker volume    │
                  │  sqlite_data      │  → /data/promptevo.db
                  └───────────────────┘
```

**Two services, one volume:**

- **`backend`** — a single CGO-free Go binary. Runs the chi HTTP router (`/api/*`
  plus `/healthz`), the per-run experiment orchestrator goroutine, the agent and
  reflector LLM loops, the pure Wordle game logic, and the SQLite persistence
  layer. Talks to OpenRouter over HTTPS using your API key. Listens on `:8080`
  inside the Compose network only (not published to the host).
- **`frontend`** — the React/TypeScript bundle built with Vite and served by
  nginx. nginx also reverse-proxies `/api/*` to the backend, with buffering
  disabled so Server-Sent Events stream to the browser uninterrupted. This is the
  only service published to the host (port **3001**).
- **`sqlite_data`** — a named Docker volume holding the SQLite database, so runs
  persist across `docker compose down`/`up`.

---

## 4. Quickstart

```bash
git clone <repo-url>
cd promptevo
cp .env.example .env
# Edit .env:
#   - Set LLM_PROVIDER and the matching API key (see "Switching providers" below)
#   - Set AUTH_USERNAME + AUTH_PASSWORD to protect the app (recommended)
docker compose up -d
# Open http://localhost:3001
```

By default `docker compose up` **pulls the pre-built images from Docker Hub**
(`vpoluyaktov/promptevo-backend:latest` and `vpoluyaktov/promptevo-frontend:latest`)
— no local build step. The backend comes up first; the frontend waits for the
backend's health check to pass before starting (see
`depends_on: condition: service_healthy`).

Local builds are only needed for **development** when you want to run your own code
changes — see §12 (Development setup) for the native edit loop, or use
`docker compose up --build` to build the images from source instead of pulling them.

> The app boots even **without** an API key — you can browse the UI and past runs.
> Starting a *new* run requires the active provider's API key; without it `POST /api/runs`
> returns `503 LLM gateway not configured`.

---

## 5. Authentication

When `AUTH_USERNAME` and `AUTH_PASSWORD` are set in `.env`, the app requires a
login before any API call can be made. A login form is shown automatically; after
signing in, the token is stored in `localStorage` and included on every request.

- **Enable** (recommended for public deployments): set both vars in `.env`.
- **Disable** (local dev / trusted network): leave both vars blank or omit them.
- The token is stateless and derived from your credentials + a random server secret.
  It is invalidated on every container restart, requiring a fresh login.
- There is no "forgot password" flow — just update `.env` and restart.

---

## 6. Switching LLM providers

The app supports three providers. Only one is active at a time, selected by
`LLM_PROVIDER` in `.env`:

| Provider | `LLM_PROVIDER` | Key variable | Model example |
|---|---|---|---|
| OpenRouter (default) | `openrouter` | `OPENROUTER_API_KEY` | `openai/gpt-4o` |
| Anthropic direct | `anthropic` | `ANTHROPIC_API_KEY` | `claude-sonnet-4-6` |
| OpenAI direct | `openai` | `OPENAI_API_KEY` | `gpt-4o` |

To switch, edit `.env` and restart (**no rebuild needed**):

```bash
# Example: switch to Anthropic
LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...

docker compose up -d
```

The model dropdown in the UI automatically reflects the active provider's available
models. For a fair cross-model comparison, run two experiments with the **same seed**
— one per provider — then use the Model Comparison view to overlay their trajectories.

---

## 7. Running an experiment

1. **Open the UI** at <http://localhost:3000>. The landing page is the **Runs
   list** — every run you have started, newest first, with its models, config, and
   headline metrics.
2. **Click "New Run"** to open the configuration form. Fields:
   - **Player model** — the model that actually guesses words.
   - **Reflector model** — the model that rewrites the strategy prompt between
     generations. Set this to a stronger model to test whether a good coach lifts a
     weaker player.
   - **Temperature** (`0.0–2.0`, default `0.7`).
   - **Seed** (default `42`) — fixes the sampled words and their order.
   - **Generations** (`1–50`) — how many play→reflect cycles to run.
   - **Games per generation** (`1–500`) — sample size behind each generation's
     metrics. More games = less noise, more API cost.
   - **Word sample size** (`1–`len(answers)) — how many answers to draw from the
     pool; the same sample is replayed every generation.
   - **Include baselines** — also play the random / frequency / entropy reference
     players in generation 0, so the dashboards can overlay a floor and ceiling.
3. **Submit.** The run is created (`pending`), the orchestrator goroutine starts
   (`running`), and you are taken to the **Run detail** view.
4. **Watch it live.** While the run is `running`, the detail view opens an
   `EventSource` to `/api/runs/{id}/stream`. The **live feed** animates each guess
   on a Wordle board as it is scored; charts update at the end of every generation;
   the view closes the stream automatically on `run_end`.
5. **Review when done.** Once `completed`, the same view shows the full historical
   record: per-generation metric charts, the evolving prompt panels, and
   per-generation game tables. Re-opening a finished run replays its final state
   from the database (no live stream needed).

---

## 8. Interpreting the dashboards

- **Solve rate** — the fraction of games won (answer found within six guesses) in a
  generation, in `[0, 1]`. This is the primary "is the agent getting better?" axis.
  Read it against the baselines: the entropy player is a strong reference ceiling,
  random is the floor.
- **Mean guesses** — average number of guesses taken per game. Lower is better, and
  it can keep improving even after solve rate saturates (solving in 3.8 vs 4.5
  guesses is real progress the solve rate alone hides).
- **Information gain** (bits) — for each guess, `log2(candidates_before /
  candidates_after)`: how much the guess narrowed the space of possible answers. A
  guess that eliminates nothing scores ~0 bits; the final correct guess scores
  `log2(remaining)`. **Why it matters:** it measures guess *quality* independent of
  luck — a generation can win the same number of games while playing far more
  efficiently, and information gain is where you see that. Per-game totals are
  averaged into `meanInfoGain`.
- **Prompt evolution timeline** — each generation stores the exact strategy prompt
  it used. The prompt panels (and the `prompt_len` sparkline) let you watch the
  agent's self-authored strategy grow, shrink, and shift in wording across
  generations. This is the actual artifact of the meta-learning.
- **Violation rate** — mean invalid/contradictory guesses per game (e.g. reusing a
  letter it already knows is absent). A well-evolved prompt should drive this toward
  zero; a model trading rule-following for cleverness shows up as a stubbornly high
  violation rate.
- **Convergence indicator** — a pill computed over the last three generations'
  solve rates:
  - **stable** — spread `< 0.02`; the agent has settled.
  - **oscillating** — the trend reverses direction; the agent's self-edits are
    fighting each other rather than compounding.
  - **improving** — a monotonic trend beyond the stability band (also shown when
    fewer than three generations have completed, i.e. not enough data yet).

---

## 9. Cross-model comparison guide

To compare two models fairly, hold *everything except the model* constant:

1. Start a run with **model A** as the player and note its **seed**, **word sample
   size**, **games per generation**, and **generations**.
2. Start a second run with **model B** as the player and the **identical** seed and
   counts. Because sampling is a seeded Fisher–Yates shuffle of the answer pool,
   both runs play the exact same words in the exact same order — an apples-to-apples
   comparison.
3. Open the two runs side by side from the Runs list and compare their metric curves
   and convergence. The interesting deltas are not just final solve rate but
   *learning speed* (how many generations to plateau) and *stability* (does B
   oscillate where A converges?).

You can vary the **reflector** independently to ask a different question: keep the
player fixed and swap the reflector to test whether a stronger "coach" produces
better self-edits for the same player.

---

## 10. Word list source

`data/answers.txt` (the answer pool) and `data/guesses.txt` (the valid-guess pool,
a superset of the answers) ship as **placeholder** curated lists of real five-letter
words. They are sufficient to exercise the full pipeline, but they are **not** the
official Wordle lists.

For publication-quality results, replace them with the canonical **NYT Wordle**
lists — **2309** answer words and **10657** valid guesses — which are widely
available in open-source Wordle repositories. The file format does not change: one
lowercase five-letter word per line, with every answer also present in the guess
list. Drop the replacements in at `data/answers.txt` and `data/guesses.txt` and
rebuild.

> **Attribution.** Wordle was created by **Josh Wardle** and acquired by **The New
> York Times** in 2022. The word lists are the property of their respective owners;
> this project bundles only placeholder dictionary words and does not redistribute
> the NYT lists.

---

## 11. Reproducibility

- **Seeded word sampling.** Each run draws `wordSampleSize` answers via a
  deterministic Fisher–Yates shuffle of the answer pool seeded by the run's `seed`.
  Identical `(seed, wordSampleSize, answer list)` ⇒ identical sample, in identical
  order. The same sample is replayed in **every** generation, so generation-to-
  generation metric deltas isolate prompt quality from word luck.
- **Re-running a stored experiment.** Every run persists its full configuration
  (models, temperature, seed, counts) in the `runs` row (`config_json`). To
  reproduce a run exactly, read its config from the Run detail view (or the API)
  and start a new run with the same values. Same config + same word lists ⇒ same
  words played in the same order. (LLM responses are not bit-for-bit deterministic
  even at temperature 0, so solve rates may vary slightly run-to-run; the *inputs*
  are fully reproducible.)

---

## 12. Development setup (without Docker)

Run the two services natively for a fast edit loop.

**Backend** (Go 1.23+):

```bash
export LLM_PROVIDER=openrouter       # or anthropic / openai
export OPENROUTER_API_KEY=sk-or-...  # set the key for your chosen provider
export DB_PATH=./promptevo.db        # local file instead of the /data volume
go run ./cmd/server                  # listens on :8080

# quality gate before committing
go vet ./...
go test ./...
golangci-lint run ./...
```

**Frontend** (Node 20):

```bash
cd frontend
npm install
npm run dev                          # Vite dev server, default http://localhost:5173
```

Point the Vite dev server's `/api` proxy at `http://localhost:8080` (in
`frontend/vite.config.ts`) so the SPA reaches the local Go backend, then open the
Vite URL. In the Docker deployment nginx fills this role, so the SPA uses relative
`/api` URLs and needs no `VITE_API_URL`.

---

## 13. Configuration reference

All backend configuration is via environment variables (env → struct, with
defaults). In Docker these are set on the `backend` service in `docker-compose.yml`
and sourced from `.env`.

| Variable               | Default                          | Description |
|------------------------|----------------------------------|-------------|
| `LLM_PROVIDER`         | `openrouter`                     | Active provider: `openrouter` \| `anthropic` \| `openai`. |
| `OPENROUTER_API_KEY`   | —                                | Required when `LLM_PROVIDER=openrouter`. |
| `ANTHROPIC_API_KEY`    | —                                | Required when `LLM_PROVIDER=anthropic`. |
| `OPENAI_API_KEY`       | —                                | Required when `LLM_PROVIDER=openai`. |
| `AUTH_USERNAME`        | —                                | Login username. Auth disabled when blank. |
| `AUTH_PASSWORD`        | —                                | Login password. Auth disabled when blank. |
| `PORT`                 | `8080`                           | HTTP listen port. |
| `DB_PATH`              | `/data/promptevo.db`             | SQLite file path (on the `sqlite_data` volume). |
| `ANSWERS_PATH`         | `data/answers.txt`               | Answer word list (relative to the working dir). |
| `GUESSES_PATH`         | `data/guesses.txt`               | Valid-guess word list. |
| `OPENROUTER_BASE_URL`  | `https://openrouter.ai/api/v1`   | Override for testing/mocks (OpenRouter only). |
| `LLM_TIMEOUT_SECONDS`  | `60`                             | Per-request LLM timeout. |
| `MAX_CONCURRENT_RUNS`  | `2`                              | Orchestrator concurrency cap. |
| `LOG_LEVEL`            | `info`                           | `debug` \| `info` \| `warn` \| `error`. |

The `.env.example` file lists the variables you are most likely to set; copy it to
`.env` and edit. The frontend is configured at build time only and needs no runtime
env in the Compose deployment.

---

## License & attribution

Wordle is a trademark of The New York Times Company; this project is an independent
research tool and is not affiliated with or endorsed by the NYT. See §7 for word
list attribution.
