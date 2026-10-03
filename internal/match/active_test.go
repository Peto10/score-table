package match

import (
	"testing"
	"time"
)

func TestTimerPauseLifecycle(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewActiveMatchStore()
	s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000, UpdatedAt: now}})
	check := func(at time.Time, running, paused bool, remaining int64) {
		t.Helper()
		snap, ok := s.TimerSnapshotNow(at)
		if !ok || snap.Running != running || snap.Paused != paused || snap.RemainingMs != remaining {
			t.Fatalf("timer snapshot = %+v, exists=%v; want running=%v paused=%v remaining=%d", snap, ok, running, paused, remaining)
		}
	}
	check(now, false, false, 60000)
	s.TimerToggleRun(now)
	check(now.Add(5*time.Second), true, false, 55000)
	s.TimerToggleRun(now.Add(5 * time.Second))
	check(now.Add(15*time.Second), false, true, 55000)
	if !s.Get().Timer.Paused {
		t.Fatal("match copy lost pause state used by audience snapshots")
	}
	s.TimerSet(now.Add(15*time.Second), 45000)
	check(now.Add(20*time.Second), false, true, 45000)
	s.TimerToggleRun(now.Add(20 * time.Second))
	check(now.Add(25*time.Second), true, false, 40000)
	check(now.Add(65*time.Second), false, false, 0)
	s.TimerToggleRun(now.Add(66 * time.Second))
	check(now.Add(66*time.Second), false, false, 0)
}

func TestTimerClearsPause(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tc := range []struct {
		name string
		act  func(*Store)
	}{
		{"reset", func(s *Store) { s.TimerReset(now) }},
		{"hide then show", func(s *Store) {
			s.TimerSetVisibility(now, false)
			s.TimerSetVisibility(now, true)
		}},
		{"set zero", func(s *Store) { s.TimerSet(now, 0) }},
		{"new match", func(s *Store) {
			s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000, UpdatedAt: now}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewActiveMatchStore()
			s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000, UpdatedAt: now}})
			s.TimerToggleRun(now)
			s.TimerToggleRun(now.Add(time.Second))
			tc.act(s)
			snap, _ := s.TimerSnapshotNow(now.Add(2 * time.Second))
			if snap.Running || snap.Paused {
				t.Fatalf("timer must be ready or stopped after %s, got %+v", tc.name, snap)
			}
		})
	}
}

func TestPauseAfterTimerExpires(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewActiveMatchStore()
	s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, UpdatedAt: now}})
	s.TimerToggleRun(now)
	s.TimerToggleRun(now.Add(2 * time.Second))
	snap, _ := s.TimerSnapshotNow(now.Add(2 * time.Second))
	if snap.Paused || snap.Running || snap.RemainingMs != 0 {
		t.Fatalf("expired timer must not be marked paused: %+v", snap)
	}
}
