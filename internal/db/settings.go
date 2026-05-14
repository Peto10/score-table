package db

import (
	"context"
	"database/sql"
)

type TimerSettings struct {
	DefaultMinutes int
	DefaultSeconds int
	ShowByDefault  bool
}

func GetTimerSettings(ctx context.Context, db *sql.DB) (TimerSettings, error) {
	var (
		mins int
		secs int
		show int
	)
	err := db.QueryRowContext(ctx, `
SELECT timer_default_minutes, timer_default_seconds, timer_show_by_default
FROM app_settings
WHERE id = 1
`).Scan(&mins, &secs, &show)
	if err != nil {
		return TimerSettings{}, err
	}
	return TimerSettings{
		DefaultMinutes: mins,
		DefaultSeconds: secs,
		ShowByDefault:  show != 0,
	}, nil
}

func UpdateTimerSettings(ctx context.Context, db *sql.DB, s TimerSettings) error {
	show := 0
	if s.ShowByDefault {
		show = 1
	}
	_, err := db.ExecContext(ctx, `
UPDATE app_settings
SET timer_default_minutes = ?,
    timer_default_seconds = ?,
    timer_show_by_default = ?
WHERE id = 1
`, s.DefaultMinutes, s.DefaultSeconds, show)
	return err
}

