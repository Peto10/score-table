package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func OpenAndInit(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS matches (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  team1_id TEXT NOT NULL,
  team1_name TEXT NOT NULL,
  team2_id TEXT NOT NULL,
  team2_name TEXT NOT NULL,
  team1_score INTEGER NOT NULL,
  team2_score INTEGER NOT NULL,
  started_at TEXT NOT NULL,
  ended_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS match_player_goals (
  match_id INTEGER NOT NULL,
  player_id TEXT NOT NULL,
  player_name TEXT NOT NULL,
  scoring_team_id TEXT NOT NULL,
  scoring_team_name TEXT NOT NULL,
  opponent_team_id TEXT NOT NULL,
  opponent_team_name TEXT NOT NULL,
  goals INTEGER NOT NULL,
  PRIMARY KEY (match_id, player_id, opponent_team_id),
  FOREIGN KEY (match_id) REFERENCES matches(id) ON DELETE CASCADE
);
`)
	return err
}
