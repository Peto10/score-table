package db

import (
	"context"
	"database/sql"
	"time"
)

type InsertMatchParams struct {
	Team1ID    string
	Team1Name  string
	Team2ID    string
	Team2Name  string
	Team1Score int
	Team2Score int
	StartedAt  time.Time
	EndedAt    time.Time
}

func InsertMatch(ctx context.Context, db *sql.DB, p InsertMatchParams) (int64, error) {
	res, err := db.ExecContext(ctx, `
INSERT INTO matches (
  team1_id, team1_name,
  team2_id, team2_name,
  team1_score, team2_score,
  started_at, ended_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`,
		p.Team1ID, p.Team1Name,
		p.Team2ID, p.Team2Name,
		p.Team1Score, p.Team2Score,
		p.StartedAt.UTC().Format("2006-01-02 15:04:05"),
		p.EndedAt.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type PlayerGoalRow struct {
	PlayerID         string
	PlayerName       string
	ScoringTeamID    string
	ScoringTeamName  string
	OpponentTeamID   string
	OpponentTeamName string
	Goals            int
}

func InsertMatchPlayerGoals(ctx context.Context, db *sql.DB, matchID int64, rows []PlayerGoalRow) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO match_player_goals (
  match_id,
  player_id, player_name,
  scoring_team_id, scoring_team_name,
  opponent_team_id, opponent_team_name,
  goals
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			matchID,
			r.PlayerID, r.PlayerName,
			r.ScoringTeamID, r.ScoringTeamName,
			r.OpponentTeamID, r.OpponentTeamName,
			r.Goals,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

type MatchRow struct {
	ID         int64
	Team1ID    string
	Team1Name  string
	Team2ID    string
	Team2Name  string
	Team1Score int
	Team2Score int
	StartedAt  string
	EndedAt    string
}

func ListMatches(ctx context.Context, db *sql.DB) ([]MatchRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  id,
  team1_id, team1_name,
  team2_id, team2_name,
  team1_score, team2_score,
  started_at, ended_at
FROM matches
ORDER BY ended_at DESC, id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MatchRow
	for rows.Next() {
		var m MatchRow
		if err := rows.Scan(
			&m.ID,
			&m.Team1ID, &m.Team1Name,
			&m.Team2ID, &m.Team2Name,
			&m.Team1Score, &m.Team2Score,
			&m.StartedAt, &m.EndedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func DeleteMatch(ctx context.Context, db *sql.DB, matchID int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM matches WHERE id = ?`, matchID)
	return err
}

// PlayerStats returns goals grouped by (player_id, opponent_team_id) and total per player.
func PlayerStats(ctx context.Context, db *sql.DB) (byPlayer map[string]map[string]int, totals map[string]int, err error) {
	rows, err := db.QueryContext(ctx, `
SELECT player_id, opponent_team_id, SUM(goals) AS total_goals
FROM match_player_goals
GROUP BY player_id, opponent_team_id
`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	byPlayer = map[string]map[string]int{}
	totals = map[string]int{}

	for rows.Next() {
		var playerID, oppTeamID string
		var goals int
		if err := rows.Scan(&playerID, &oppTeamID, &goals); err != nil {
			return nil, nil, err
		}
		m, ok := byPlayer[playerID]
		if !ok {
			m = map[string]int{}
			byPlayer[playerID] = m
		}
		m[oppTeamID] = goals
		totals[playerID] += goals
	}
	return byPlayer, totals, rows.Err()
}
