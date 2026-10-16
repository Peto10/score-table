package db

import (
	"context"
	"database/sql"
)

type TeamRecord struct {
	Wins   int
	Losses int
	Draws  int
}

func ListTeamRecords(ctx context.Context, db *sql.DB) (map[string]TeamRecord, error) {
	rows, err := db.QueryContext(ctx, `
SELECT team_id,
  SUM(CASE WHEN goals_for > goals_against THEN 1 ELSE 0 END),
  SUM(CASE WHEN goals_for < goals_against THEN 1 ELSE 0 END),
  SUM(CASE WHEN goals_for = goals_against THEN 1 ELSE 0 END)
FROM (
  SELECT team1_id AS team_id, team1_score AS goals_for, team2_score AS goals_against
  FROM matches
  UNION ALL
  SELECT team2_id, team2_score, team1_score
  FROM matches
)
GROUP BY team_id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make(map[string]TeamRecord)
	for rows.Next() {
		var teamID string
		var record TeamRecord
		if err := rows.Scan(&teamID, &record.Wins, &record.Losses, &record.Draws); err != nil {
			return nil, err
		}
		records[teamID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
