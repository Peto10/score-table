package http

import (
	"testing"
	"time"

	"score-table/internal/match"
)

func TestBuildSnapshotPauseVisibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		timer  match.Timer
		paused bool
	}{
		{"ready", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000}, false},
		{"paused", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 55000, Paused: true}, true},
		{"hidden", match.Timer{DefaultMs: 60000, RemainingMs: 55000, Paused: true}, false},
		{"zero", match.Timer{Show: true, DefaultMs: 60000, Paused: true}, false},
		{"unsupported", match.Timer{Show: true, RemainingMs: 55000, Paused: true}, false},
		{"expired", match.Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, Running: true, UpdatedAt: time.Now().Add(-time.Minute)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := buildSnapshot(&match.ActiveMatch{Timer: tc.timer})
			if snap.TimerPaused != tc.paused {
				t.Fatalf("audience paused state = %v; want %v for %s", snap.TimerPaused, tc.paused, tc.name)
			}
		})
	}
}
