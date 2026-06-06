package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.up.sql
var migrationSQL string

// SQLiteStore is the modernc.org/sqlite-backed implementation of Store.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at dbPath, sets recommended
// pragmas, and returns a ready-to-use SQLiteStore.
// Call Migrate to apply schema before use.
func Open(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}

	// Single connection avoids WAL reader/writer contention and ensures
	// PRAGMA statements stick for the lifetime of the pool.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("exec %q: %w", p, err)
		}
	}

	return &SQLiteStore{db: db}, nil
}

// Migrate applies the initial schema (idempotent via IF NOT EXISTS) and runs
// incremental ALTER TABLE statements for columns added after the initial deploy.
func (s *SQLiteStore) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrationSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Add max_guesses column to runs (ignored if it already exists).
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE runs ADD COLUMN max_guesses INTEGER NOT NULL DEFAULT 4`)
	// Add token-split columns to generations (ignored if they already exist).
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE generations ADD COLUMN player_tokens INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE generations ADD COLUMN reflector_tokens INTEGER NOT NULL DEFAULT 0`)
	return nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// ─── runs ────────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateRun(ctx context.Context, r *Run) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO runs
			(player_model, reflector_model, temperature, seed,
			 generations, games_per_gen, word_sample_size, max_guesses, status, config_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.PlayerModel, r.ReflectorModel, r.Temperature, r.Seed,
		r.Generations, r.GamesPerGen, r.WordSampleSize, r.MaxGuesses, r.Status, r.ConfigJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("create run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (s *SQLiteStore) GetRun(ctx context.Context, id int64) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, created_at, player_model, reflector_model, temperature, seed,
		       generations, games_per_gen, word_sample_size, max_guesses, status, config_json
		FROM runs WHERE id = ?`, id)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *SQLiteStore) ListRuns(ctx context.Context) ([]*Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, created_at, player_model, reflector_model, temperature, seed,
		       generations, games_per_gen, word_sample_size, max_guesses, status, config_json
		FROM runs ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []*Run{}
	}
	return runs, nil
}

func (s *SQLiteStore) UpdateRunStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteRun deletes a run and all child rows in a single transaction,
// performing ordered child-first deletes (guesses → games → generations → run).
func (s *SQLiteStore) DeleteRun(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	steps := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM guesses WHERE game_id IN (SELECT id FROM games WHERE run_id = ?)`, []any{id}},
		{`DELETE FROM games WHERE run_id = ?`, []any{id}},
		{`DELETE FROM generations WHERE run_id = ?`, []any{id}},
		{`DELETE FROM runs WHERE id = ?`, []any{id}},
	}
	for _, step := range steps {
		if _, err = tx.ExecContext(ctx, step.query, step.args...); err != nil {
			return fmt.Errorf("delete step %q: %w", step.query, err)
		}
	}

	return tx.Commit()
}

// scanner is the common interface shared by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanRun(s scanner) (*Run, error) {
	r := &Run{}
	err := s.Scan(
		&r.ID, &r.CreatedAt,
		&r.PlayerModel, &r.ReflectorModel, &r.Temperature, &r.Seed,
		&r.Generations, &r.GamesPerGen, &r.WordSampleSize, &r.MaxGuesses, &r.Status,
		&r.ConfigJSON,
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ─── generations ─────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateGeneration(ctx context.Context, g *Generation) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO generations (run_id, gen_index, prompt_text, prompt_len, tokens_used)
		VALUES (?, ?, ?, ?, ?)`,
		g.RunID, g.GenIndex, g.PromptText, g.PromptLen, g.TokensUsed,
	)
	if err != nil {
		return 0, fmt.Errorf("create generation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *SQLiteStore) UpdateGenerationStats(ctx context.Context, g *Generation) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations
		SET reflection_text  = ?,
		    solve_rate        = ?,
		    mean_guesses      = ?,
		    mean_info_gain    = ?,
		    violation_rate    = ?,
		    tokens_used       = ?,
		    player_tokens     = ?,
		    reflector_tokens  = ?
		WHERE id = ?`,
		g.ReflectionText, g.SolveRate, g.MeanGuesses, g.MeanInfoGain,
		g.ViolationRate, g.TokensUsed, g.PlayerTokens, g.ReflectorTokens, g.ID,
	)
	return err
}

func (s *SQLiteStore) ListGenerations(ctx context.Context, runID int64) ([]*Generation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, gen_index, prompt_text, prompt_len,
		       reflection_text, solve_rate, mean_guesses, mean_info_gain,
		       violation_rate, tokens_used, player_tokens, reflector_tokens
		FROM generations
		WHERE run_id = ?
		ORDER BY gen_index ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("list generations: %w", err)
	}
	defer rows.Close()

	var gens []*Generation
	for rows.Next() {
		g := &Generation{}
		if err := rows.Scan(
			&g.ID, &g.RunID, &g.GenIndex, &g.PromptText, &g.PromptLen,
			&g.ReflectionText, &g.SolveRate, &g.MeanGuesses, &g.MeanInfoGain,
			&g.ViolationRate, &g.TokensUsed, &g.PlayerTokens, &g.ReflectorTokens,
		); err != nil {
			return nil, err
		}
		gens = append(gens, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if gens == nil {
		gens = []*Generation{}
	}
	return gens, nil
}

// ─── games ───────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateGame(ctx context.Context, g *Game) (int64, error) {
	won := 0
	if g.Won {
		won = 1
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO games
			(run_id, gen_index, answer, won, num_guesses, info_gain_total, violations, agent_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		g.RunID, g.GenIndex, g.Answer, won, g.NumGuesses,
		g.InfoGainTotal, g.Violations, g.AgentType,
	)
	if err != nil {
		return 0, fmt.Errorf("create game: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *SQLiteStore) UpdateGame(ctx context.Context, g *Game) error {
	won := 0
	if g.Won {
		won = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE games
		SET won = ?, num_guesses = ?, info_gain_total = ?, violations = ?
		WHERE id = ?`,
		won, g.NumGuesses, g.InfoGainTotal, g.Violations, g.ID,
	)
	return err
}

func (s *SQLiteStore) ListGames(ctx context.Context, runID int64, genIndex *int) ([]*Game, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if genIndex == nil {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, run_id, gen_index, answer, won, num_guesses,
			       info_gain_total, violations, agent_type
			FROM games WHERE run_id = ? ORDER BY id ASC`, runID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, run_id, gen_index, answer, won, num_guesses,
			       info_gain_total, violations, agent_type
			FROM games WHERE run_id = ? AND gen_index = ? ORDER BY id ASC`,
			runID, *genIndex)
	}
	if err != nil {
		return nil, fmt.Errorf("list games: %w", err)
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		g := &Game{}
		var won int
		if err := rows.Scan(
			&g.ID, &g.RunID, &g.GenIndex, &g.Answer, &won,
			&g.NumGuesses, &g.InfoGainTotal, &g.Violations, &g.AgentType,
		); err != nil {
			return nil, err
		}
		g.Won = won != 0
		games = append(games, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if games == nil {
		games = []*Game{}
	}
	return games, nil
}

// ─── guesses ─────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateGuess(ctx context.Context, gu *Guess) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO guesses
			(game_id, turn_index, guess, feedback, info_gain_bits, reasoning_text)
		VALUES (?, ?, ?, ?, ?, ?)`,
		gu.GameID, gu.TurnIndex, gu.Guess, gu.Feedback,
		gu.InfoGainBits, gu.ReasoningText,
	)
	if err != nil {
		return 0, fmt.Errorf("create guess: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ─── analytics ───────────────────────────────────────────────────────────────

func (s *SQLiteStore) GameOutcomeCounts(ctx context.Context, runID int64) ([]OutcomeCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT gen_index, won, num_guesses, COUNT(*) AS cnt
		FROM games
		WHERE run_id = ? AND agent_type = 'llm'
		GROUP BY gen_index, won, num_guesses
		ORDER BY gen_index ASC, won ASC, num_guesses ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("game outcome counts: %w", err)
	}
	defer rows.Close()

	out := make([]OutcomeCount, 0)
	for rows.Next() {
		var oc OutcomeCount
		var won int
		if err := rows.Scan(&oc.GenIndex, &won, &oc.NumGuesses, &oc.Count); err != nil {
			return nil, err
		}
		oc.Won = won != 0
		out = append(out, oc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) TurnInfoGainStats(ctx context.Context, runID int64) ([]TurnInfoGainStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.gen_index, gu.turn_index,
		       AVG(gu.info_gain_bits) AS mean_ig,
		       COUNT(*) AS n
		FROM guesses gu
		JOIN games g ON gu.game_id = g.id
		WHERE g.run_id = ? AND g.agent_type = 'llm'
		GROUP BY g.gen_index, gu.turn_index
		ORDER BY g.gen_index ASC, gu.turn_index ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("turn info gain stats: %w", err)
	}
	defer rows.Close()

	out := make([]TurnInfoGainStat, 0)
	for rows.Next() {
		var t TurnInfoGainStat
		if err := rows.Scan(&t.GenIndex, &t.TurnIndex, &t.MeanInfoGain, &t.N); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) OpeningWordCounts(ctx context.Context, runID int64) ([]OpeningWordCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.gen_index, gu.guess, COUNT(*) AS cnt
		FROM guesses gu
		JOIN games g ON gu.game_id = g.id
		WHERE g.run_id = ? AND g.agent_type = 'llm' AND gu.turn_index = 0
		GROUP BY g.gen_index, gu.guess
		ORDER BY g.gen_index ASC, cnt DESC, gu.guess ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("opening word counts: %w", err)
	}
	defer rows.Close()

	out := make([]OpeningWordCount, 0)
	for rows.Next() {
		var ow OpeningWordCount
		if err := rows.Scan(&ow.GenIndex, &ow.Guess, &ow.Count); err != nil {
			return nil, err
		}
		out = append(out, ow)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) ReasoningVerbosityStats(ctx context.Context, runID int64) ([]ReasoningStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.gen_index, g.won, g.num_guesses,
		       COALESCE(SUM(LENGTH(gu.reasoning_text)), 0) AS chars
		FROM games g
		LEFT JOIN guesses gu ON gu.game_id = g.id
		WHERE g.run_id = ? AND g.agent_type = 'llm'
		GROUP BY g.id
		ORDER BY g.gen_index ASC, g.id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("reasoning verbosity stats: %w", err)
	}
	defer rows.Close()

	out := make([]ReasoningStat, 0)
	for rows.Next() {
		var rs ReasoningStat
		var won int
		var chars int
		if err := rows.Scan(&rs.GameID, &rs.GenIndex, &won, &rs.NumGuesses, &chars); err != nil {
			return nil, err
		}
		rs.Won = won != 0
		rs.ReasoningChars = chars
		out = append(out, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) WordDifficultyStats(ctx context.Context, runID int64) ([]WordDifficultyStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT answer, COUNT(*) AS games, SUM(won) AS wins
		FROM games
		WHERE run_id = ? AND agent_type = 'llm'
		GROUP BY answer
		ORDER BY (CAST(SUM(won) AS REAL) / COUNT(*)) ASC, answer ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("word difficulty stats: %w", err)
	}
	defer rows.Close()

	out := make([]WordDifficultyStat, 0)
	for rows.Next() {
		var wd WordDifficultyStat
		if err := rows.Scan(&wd.Answer, &wd.Games, &wd.Wins); err != nil {
			return nil, err
		}
		out = append(out, wd)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) ListGuesses(ctx context.Context, gameID int64) ([]*Guess, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, game_id, turn_index, guess, feedback, info_gain_bits, reasoning_text
		FROM guesses WHERE game_id = ? ORDER BY turn_index ASC`, gameID)
	if err != nil {
		return nil, fmt.Errorf("list guesses: %w", err)
	}
	defer rows.Close()

	var guesses []*Guess
	for rows.Next() {
		gu := &Guess{}
		if err := rows.Scan(
			&gu.ID, &gu.GameID, &gu.TurnIndex, &gu.Guess,
			&gu.Feedback, &gu.InfoGainBits, &gu.ReasoningText,
		); err != nil {
			return nil, err
		}
		guesses = append(guesses, gu)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if guesses == nil {
		guesses = []*Guess{}
	}
	return guesses, nil
}
