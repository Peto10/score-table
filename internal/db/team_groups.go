package db

import (
	"context"
	"database/sql"
)

type TeamGroupRow struct {
	ID   int64
	Name string
}

func ListTeamGroups(ctx context.Context, db *sql.DB) ([]TeamGroupRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM team_groups ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make([]TeamGroupRow, 0)
	for rows.Next() {
		var group TeamGroupRow
		if err := rows.Scan(&group.ID, &group.Name); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}

func ListTeamGroupMemberships(ctx context.Context, db *sql.DB) (map[int64]int64, error) {
	rows, err := db.QueryContext(ctx, `
SELECT m.team_id, m.group_id
FROM team_group_memberships m
JOIN teams t ON t.id = m.team_id
JOIN team_groups g ON g.id = m.group_id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memberships := make(map[int64]int64)
	for rows.Next() {
		var teamID, groupID int64
		if err := rows.Scan(&teamID, &groupID); err != nil {
			return nil, err
		}
		memberships[teamID] = groupID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memberships, nil
}

func CreateTeamGroup(ctx context.Context, db *sql.DB, name string) (int64, error) {
	res, err := db.ExecContext(ctx, `INSERT INTO team_groups (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func DeleteTeamGroup(ctx context.Context, db *sql.DB, groupID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Explicit cleanup also works on pooled connections without foreign keys enabled.
	if _, err := tx.ExecContext(ctx, `DELETE FROM team_group_memberships WHERE group_id = ?`, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM team_groups WHERE id = ?`, groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func setTeamGroup(ctx context.Context, tx *sql.Tx, teamID, groupID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM team_group_memberships WHERE team_id = ?`, teamID); err != nil {
		return err
	}
	if groupID == 0 {
		return nil
	}
	// A stale form referencing a deleted group safely leaves the team ungrouped.
	_, err := tx.ExecContext(ctx, `
INSERT INTO team_group_memberships (team_id, group_id)
SELECT t.id, g.id FROM teams t CROSS JOIN team_groups g
WHERE t.id = ? AND g.id = ?
`, teamID, groupID)
	return err
}
