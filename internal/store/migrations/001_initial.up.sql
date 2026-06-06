CREATE TABLE IF NOT EXISTS runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    player_model TEXT NOT NULL,
    reflector_model TEXT NOT NULL,
    temperature REAL NOT NULL DEFAULT 0.7,
    seed INTEGER NOT NULL,
    generations INTEGER NOT NULL,
    games_per_gen INTEGER NOT NULL,
    word_sample_size INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    config_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS generations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES runs(id),
    gen_index INTEGER NOT NULL,
    prompt_text TEXT NOT NULL,
    prompt_len INTEGER NOT NULL,
    reflection_text TEXT,
    solve_rate REAL,
    mean_guesses REAL,
    mean_info_gain REAL,
    violation_rate REAL,
    tokens_used INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS games (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id INTEGER NOT NULL REFERENCES runs(id),
    gen_index INTEGER NOT NULL,
    answer TEXT NOT NULL,
    won INTEGER NOT NULL DEFAULT 0,
    num_guesses INTEGER NOT NULL DEFAULT 0,
    info_gain_total REAL NOT NULL DEFAULT 0,
    violations INTEGER NOT NULL DEFAULT 0,
    agent_type TEXT NOT NULL DEFAULT 'llm'
);

CREATE TABLE IF NOT EXISTS guesses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games(id),
    turn_index INTEGER NOT NULL,
    guess TEXT NOT NULL,
    feedback TEXT NOT NULL,
    info_gain_bits REAL NOT NULL DEFAULT 0,
    reasoning_text TEXT
);

CREATE INDEX IF NOT EXISTS idx_generations_run ON generations(run_id, gen_index);
CREATE INDEX IF NOT EXISTS idx_games_run ON games(run_id, gen_index);
CREATE INDEX IF NOT EXISTS idx_guesses_game ON guesses(game_id, turn_index);
