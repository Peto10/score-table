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
	if snap.Paused || snap.Running || !snap.Expired || snap.RemainingMs != 0 {
		t.Fatalf("expired timer must not be marked paused: %+v", snap)
	}
}

func TestTimerExpiryLifecycle(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewActiveMatchStore()
	s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, UpdatedAt: now}})
	check := func(at time.Time, running, expired bool, remaining int64) {
		t.Helper()
		snap, ok := s.TimerSnapshotNow(at)
		if !ok || snap.Running != running || snap.Expired != expired || snap.RemainingMs != remaining {
			t.Fatalf("timer snapshot = %+v, exists=%v; want running=%v expired=%v remaining=%d", snap, ok, running, expired, remaining)
		}
	}
	check(now, false, false, 1000)
	s.TimerToggleRun(now)
	check(now.Add(999*time.Millisecond), true, false, 1)
	check(now.Add(time.Second), false, true, 0)
	check(now.Add(2*time.Second), false, true, 0)
	if !s.Get().Timer.Expired {
		t.Fatal("match copy lost expiry state used by audience reconnect snapshots")
	}
	// Pressing start/stop on an expired timer must retain its expiry state.
	s.TimerToggleRun(now.Add(3 * time.Second))
	check(now.Add(3*time.Second), false, true, 0)
	s.TimerReset(now.Add(4 * time.Second))
	check(now.Add(4*time.Second), false, false, 1000)
	s.TimerToggleRun(now.Add(4 * time.Second))
	check(now.Add(5*time.Second), false, true, 0)
}

func TestTimerClearsExpiry(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tc := range []struct {
		name string
		act  func(*Store)
	}{
		{"reset", func(s *Store) { s.TimerReset(now.Add(2 * time.Second)) }},
		{"set zero", func(s *Store) { s.TimerSet(now.Add(2*time.Second), 0) }},
		{"set positive", func(s *Store) { s.TimerSet(now.Add(2*time.Second), 5000) }},
		{"hide", func(s *Store) { s.TimerSetVisibility(now.Add(2*time.Second), false) }},
		{"show", func(s *Store) { s.TimerSetVisibility(now.Add(2*time.Second), true) }},
		{"new match", func(s *Store) {
			s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, Expired: true, UpdatedAt: now.Add(2 * time.Second)}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewActiveMatchStore()
			s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, UpdatedAt: now}})
			s.TimerToggleRun(now)
			snap, _ := s.TimerSnapshotNow(now.Add(time.Second))
			if !snap.Expired {
				t.Fatal("timer did not naturally expire")
			}
			tc.act(s)
			snap, _ = s.TimerSnapshotNow(now.Add(2 * time.Second))
			if snap.Expired || s.Get().Timer.Expired {
				t.Fatalf("%s must clear expiry: %+v", tc.name, snap)
			}
		})
	}
}

func TestTimerZeroDoesNotExpire(t *testing.T) {
	now := time.Unix(100, 0)
	for _, running := range []bool{false, true} {
		for _, toggle := range []bool{false, true} {
			s := NewActiveMatchStore()
			s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 1000, Running: running, UpdatedAt: now}})
			if toggle {
				s.TimerToggleRun(now.Add(time.Second))
			}
			snap, _ := s.TimerSnapshotNow(now.Add(time.Second))
			if snap.Expired || snap.Running || snap.Paused || snap.RemainingMs != 0 {
				t.Fatalf("a timer already at zero must stay unexpired (running=%v toggle=%v): %+v", running, toggle, snap)
			}
		}
	}
}

func TestTimerAdjustmentWhileRunningDoesNotExpire(t *testing.T) {
	now := time.Unix(100, 0)
	for _, remaining := range []int64{0, 5000} {
		s := NewActiveMatchStore()
		s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: 1000, RemainingMs: 1000, UpdatedAt: now}})
		s.TimerToggleRun(now)
		// Even if the old countdown has elapsed, setting time is a manual action.
		s.TimerSet(now.Add(2*time.Second), remaining)
		snap, _ := s.TimerSnapshotNow(now.Add(2 * time.Second))
		if snap.Expired || snap.RemainingMs != remaining || snap.Running != (remaining > 0) {
			t.Fatalf("manual adjustment to %d must clear expiry and preserve positive running timers: %+v", remaining, snap)
		}
	}
}

func TestTimerCanRecoverFromZero(t *testing.T) {
	now := time.Unix(100, 0)
	for _, initial := range []int64{0, 60000} {
		s := NewActiveMatchStore()
		s.Start(&ActiveMatch{Timer: Timer{Show: true, DefaultMs: initial, RemainingMs: initial, UpdatedAt: now}})
		for _, action := range []func(time.Time) bool{
			func(at time.Time) bool { return s.TimerSet(at, 0) }, s.TimerToggleRun, s.TimerReset,
		} {
			if !action(now) {
				t.Fatalf("zero timer must accept set/toggle/reset (initial=%d)", initial)
			}
			snap, _ := s.TimerSnapshotNow(now.Add(time.Second))
			if !snap.Show || snap.DefaultMs != 0 || snap.RemainingMs != 0 || snap.Running || snap.Paused || snap.Expired {
				t.Fatalf("zero timer must stay visible, stopped, and unexpired: %+v", snap)
			}
		}
		if !s.TimerSet(now.Add(time.Second), 90000) || !s.TimerToggleRun(now.Add(time.Second)) {
			t.Fatal("timer could not be configured and started after zero")
		}
		s.TimerToggleRun(now.Add(2 * time.Second))
		snap, _ := s.TimerSnapshotNow(now.Add(2 * time.Second))
		if snap.Running || !snap.Paused || snap.RemainingMs != 89000 || snap.DefaultMs != 90000 || snap.Expired {
			t.Fatalf("recovered timer did not pause correctly: %+v", snap)
		}
		s.TimerReset(now.Add(3 * time.Second))
		snap, _ = s.TimerSnapshotNow(now.Add(3 * time.Second))
		if snap.Running || snap.Paused || snap.Expired || snap.RemainingMs != 90000 {
			t.Fatalf("recovered timer did not reset correctly: %+v", snap)
		}
	}
}
