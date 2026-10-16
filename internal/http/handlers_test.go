package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"score-table/internal/match"
)

func TestBuildSnapshotPauseVisibility(t *testing.T) {
	for _, tc := range []struct {
		name  string
		timer match.Timer
		state string
	}{
		{"ready", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000}, "paused"},
		{"explicit pause", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 55000, Paused: true}, "paused"},
		{"hidden", match.Timer{DefaultMs: 60000, RemainingMs: 55000, Paused: true}, "hidden"},
		{"manual zero", match.Timer{Show: true}, "paused"},
		{"running zero", match.Timer{Show: true, Running: true, UpdatedAt: time.Now().Add(-time.Minute)}, "paused"},
		{"zero default with remaining time", match.Timer{Show: true, RemainingMs: 55000}, "paused"},
		{"running", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000, Running: true, UpdatedAt: time.Now()}, "running"},
		{"naturally elapsed", match.Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, Running: true, UpdatedAt: time.Now().Add(-time.Minute)}, "expired"},
		{"normalized expiry", match.Timer{Show: true, DefaultMs: 1000, Expired: true}, "expired"},
		{"hidden expiry", match.Timer{Expired: true}, "hidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := buildSnapshot(&match.ActiveMatch{Timer: tc.timer})
			if snap.ShowTimer != tc.timer.Show {
				t.Fatalf("timer visibility must follow Show, regardless of duration: %+v", snap)
			}
			// Exactly one public status is active when shown; all are off when hidden.
			for state, active := range map[string]bool{"running": snap.TimerRunning, "paused": snap.TimerPaused, "expired": snap.TimerExpired} {
				if active != (tc.state == state) {
					t.Fatalf("audience timer must be %s for %s: %+v", tc.state, tc.name, snap)
				}
			}
		})
	}
}

func TestBuildSnapshotExpiry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timer   match.Timer
		expired bool
	}{
		{"ready", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000}, false},
		{"paused", match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 55000, Paused: true}, false},
		{"manual zero", match.Timer{Show: true, DefaultMs: 60000}, false},
		{"running zero", match.Timer{Show: true, DefaultMs: 60000, Running: true, UpdatedAt: time.Now().Add(-time.Minute)}, false},
		{"naturally elapsed", match.Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, Running: true, UpdatedAt: time.Now().Add(-time.Minute)}, true},
		{"normalized expiry", match.Timer{Show: true, DefaultMs: 1000, Expired: true}, true},
		{"positive time", match.Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, Expired: true}, false},
		{"hidden", match.Timer{DefaultMs: 1000, Expired: true}, false},
		{"zero default", match.Timer{Show: true}, false},
		{"stored expiry with zero default", match.Timer{Show: true, Expired: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := buildSnapshot(&match.ActiveMatch{Timer: tc.timer})
			if snap.TimerExpired != tc.expired {
				t.Fatalf("audience expiry state = %v; want %v for %s", snap.TimerExpired, tc.expired, tc.name)
			}
		})
	}
}

func TestSnapshotExpirySurvivesAdminReadAndReconnect(t *testing.T) {
	s := match.NewActiveMatchStore()
	s.Start(&match.ActiveMatch{Timer: match.Timer{
		Show: true, DefaultMs: 1000, RemainingMs: 1000,
		Running: true, UpdatedAt: time.Now().Add(-time.Minute),
	}})
	h := &Handlers{active: s}
	checkEvent := func(expired bool, remaining int64) {
		t.Helper()
		event, ok := h.snapshotEvent()
		if !ok {
			t.Fatal("active match has no audience snapshot")
		}
		data := strings.TrimSuffix(strings.TrimPrefix(string(event), "event: snapshot\ndata: "), "\n\n")
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &fields); err != nil {
			t.Fatalf("invalid snapshot event: %v", err)
		}
		if _, ok := fields["timerExpired"]; !ok {
			t.Fatal("snapshot JSON does not expose timerExpired")
		}
		var snap snapshot
		if err := json.Unmarshal([]byte(data), &snap); err != nil {
			t.Fatalf("invalid snapshot fields: %v", err)
		}
		if !snap.ShowTimer || snap.TimerRunning || snap.TimerPaused != !expired || snap.TimerExpired != expired || snap.TimerRemainingMs != remaining {
			t.Fatalf("audience snapshot = %+v; want expired=%v remaining=%d", snap, expired, remaining)
		}
	}
	// Audience reads detect natural expiry even before the admin reads the timer.
	checkEvent(true, 0)
	// Opening/refreshing the admin normalizes the timer to a stopped zero timer.
	now := time.Now()
	timer, _ := s.TimerSnapshotNow(now)
	if timer.Running || !timer.Expired {
		t.Fatalf("admin timer read did not retain expiry: %+v", timer)
	}
	// Reconnecting displays still receive timeout after normalization.
	checkEvent(true, 0)
	checkEvent(true, 0)
	s.TimerReset(now)
	checkEvent(false, 1000)
	s.TimerToggleRun(now)
	s.TimerToggleRun(now)
	checkEvent(false, 1000)
	s.TimerSet(now, 0)
	checkEvent(false, 0)
}
