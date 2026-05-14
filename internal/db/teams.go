package db

import (
	"context"
	"database/sql"
)

type TeamRow struct {
	ID   int64
	Name string
}

type PlayerRow struct {
	ID     int64
	TeamID int64
	Slot   int
	Name   string
}

type TeamWithPlayers struct {
	TeamRow
	Players []PlayerRow
}

func ListTeamsWithPlayers(ctx context.Context, db *sql.DB) ([]TeamWithPlayers, error) {
	rows, err := db.QueryContext(ctx, `
SELECT
  t.id, t.name,
  p.id, p.team_id, p.slot, p.name
FROM teams t
LEFT JOIN players p ON p.team_id = t.id
ORDER BY t.id ASC, p.slot ASC, p.id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTeam := map[int64]*TeamWithPlayers{}
	order := make([]int64, 0)

	for rows.Next() {
		var (
			tid   int64
			tname string

			pid    sql.NullInt64
			pteam  sql.NullInt64
			pslot  sql.NullInt64
			pname  sql.NullString
		)
		if err := rows.Scan(&tid, &tname, &pid, &pteam, &pslot, &pname); err != nil {
			return nil, err
		}

		t, ok := byTeam[tid]
		if !ok {
			t = &TeamWithPlayers{TeamRow: TeamRow{ID: tid, Name: tname}, Players: []PlayerRow{}}
			byTeam[tid] = t
			order = append(order, tid)
		}
		// Left join: player columns can be NULL when a team has no players yet.
		if pid.Valid {
			t.Players = append(t.Players, PlayerRow{
				ID:     pid.Int64,
				TeamID: pteam.Int64,
				Slot:   int(pslot.Int64),
				Name:   pname.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]TeamWithPlayers, 0, len(order))
	for _, tid := range order {
		out = append(out, *byTeam[tid])
	}
	return out, nil
}

func CreateTeamWithTwoPlayers(ctx context.Context, db *sql.DB, teamName, player1, player2 string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `INSERT INTO teams (name) VALUES (?)`, teamName)
	if err != nil {
		return 0, err
	}
	teamID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO players (team_id, slot, name) VALUES (?, 1, ?)`, teamID, player1); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO players (team_id, slot, name) VALUES (?, 2, ?)`, teamID, player2); err != nil {
		return 0, err
	}

	return teamID, tx.Commit()
}

func UpdateTeamAndTwoPlayers(ctx context.Context, db *sql.DB, teamID int64, teamName, player1, player2 string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE teams SET name = ? WHERE id = ?`, teamName, teamID); err != nil {
		return err
	}

	// Keep player IDs stable by updating by (team_id, slot). If a slot row is missing, insert it.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO players (team_id, slot, name) VALUES (?, 1, ?)
ON CONFLICT(team_id, slot) DO UPDATE SET name = excluded.name
`, teamID, player1); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO players (team_id, slot, name) VALUES (?, 2, ?)
ON CONFLICT(team_id, slot) DO UPDATE SET name = excluded.name
`, teamID, player2); err != nil {
		return err
	}

	return tx.Commit()
}

func DeleteTeam(ctx context.Context, db *sql.DB, teamID int64) error {
	_, err := db.ExecContext(ctx, `DELETE FROM teams WHERE id = ?`, teamID)
	return err
}

func GetTeamWithPlayers(ctx context.Context, db *sql.DB, teamID int64) (TeamWithPlayers, error) {
	var t TeamWithPlayers
	t.Players = []PlayerRow{}

	rows, err := db.QueryContext(ctx, `
SELECT
  t.id, t.name,
  p.id, p.team_id, p.slot, p.name
FROM teams t
LEFT JOIN players p ON p.team_id = t.id
WHERE t.id = ?
ORDER BY p.slot ASC, p.id ASC
`, teamID)
	if err != nil {
		return TeamWithPlayers{}, err
	}
	defer rows.Close()

	seen := false
	for rows.Next() {
		seen = true
		var (
			tid   int64
			tname string

			pid    sql.NullInt64
			pteam  sql.NullInt64
			pslot  sql.NullInt64
			pname  sql.NullString
		)
		if err := rows.Scan(&tid, &tname, &pid, &pteam, &pslot, &pname); err != nil {
			return TeamWithPlayers{}, err
		}
		t.TeamRow = TeamRow{ID: tid, Name: tname}
		if pid.Valid {
			t.Players = append(t.Players, PlayerRow{
				ID:     pid.Int64,
				TeamID: pteam.Int64,
				Slot:   int(pslot.Int64),
				Name:   pname.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return TeamWithPlayers{}, err
	}
	if !seen {
		return TeamWithPlayers{}, sql.ErrNoRows
	}
	return t, nil
}

