package match

import (
	"sync"
	"time"
)

type ActiveMatch struct {
	Team1 TeamSide
	Team2 TeamSide

	StartedAt time.Time

	// GoalsByPlayerID stores goals for this match, keyed by player_id.
	GoalsByPlayerID map[string]int

	Timer Timer
}

type Timer struct {
	Show bool

	DefaultMs   int64
	RemainingMs int64
	Running     bool
	UpdatedAt   time.Time
}

func (t *Timer) remainingAt(now time.Time) int64 {
	if t.RemainingMs <= 0 {
		return 0
	}
	if !t.Running {
		return t.RemainingMs
	}
	elapsed := now.Sub(t.UpdatedAt).Milliseconds()
	rem := t.RemainingMs - elapsed
	if rem < 0 {
		return 0
	}
	return rem
}

type TeamSide struct {
	TeamID   string
	TeamName string
	Players  []PlayerRef
}

type PlayerRef struct {
	PlayerID   string
	PlayerName string
}

type Store struct {
	mu    sync.RWMutex
	match *ActiveMatch
}

func NewActiveMatchStore() *Store {
	return &Store{}
}

func (s *Store) Get() *ActiveMatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.match == nil {
		return nil
	}
	cp := *s.match
	cp.GoalsByPlayerID = make(map[string]int, len(s.match.GoalsByPlayerID))
	for k, v := range s.match.GoalsByPlayerID {
		cp.GoalsByPlayerID[k] = v
	}
	return &cp
}

func (s *Store) Start(m *ActiveMatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.match = m
}

func (s *Store) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.match = nil
}

func (s *Store) Inc(playerID string) (ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	s.match.GoalsByPlayerID[playerID]++
	return true
}

func (s *Store) Dec(playerID string) (ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	if s.match.GoalsByPlayerID[playerID] <= 0 {
		s.match.GoalsByPlayerID[playerID] = 0
		return true
	}
	s.match.GoalsByPlayerID[playerID]--
	return true
}

type TimerSnapshot struct {
	Show        bool
	Running     bool
	RemainingMs int64
	DefaultMs   int64
	UpdatedAt   time.Time
}

func (s *Store) TimerSnapshotNow(now time.Time) (TimerSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return TimerSnapshot{}, false
	}
	t := &s.match.Timer
	rem := t.remainingAt(now)
	if t.Running && rem == 0 {
		t.Running = false
		t.RemainingMs = 0
		t.UpdatedAt = now
		rem = 0
	} else if t.Running {
		// keep state stable at last update; snapshot uses computed remaining
	}
	return TimerSnapshot{
		Show:        t.Show,
		Running:     t.Running,
		RemainingMs: rem,
		DefaultMs:   t.DefaultMs,
		UpdatedAt:   t.UpdatedAt,
	}, true
}

func (s *Store) TimerSetVisibility(now time.Time, show bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	t := &s.match.Timer
	if !show {
		t.Show = false
		t.Running = false
		t.RemainingMs = 0
		t.UpdatedAt = now
		return true
	}
	t.Show = true
	if t.DefaultMs > 0 && t.RemainingMs <= 0 {
		t.RemainingMs = t.DefaultMs
	}
	t.UpdatedAt = now
	return true
}

func (s *Store) TimerToggleRun(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	t := &s.match.Timer
	if !t.Show || t.DefaultMs <= 0 {
		return false
	}
	rem := t.remainingAt(now)
	if rem == 0 {
		t.Running = false
		t.RemainingMs = 0
		t.UpdatedAt = now
		return true
	}
	t.RemainingMs = rem
	t.Running = !t.Running
	t.UpdatedAt = now
	return true
}

func (s *Store) TimerReset(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	t := &s.match.Timer
	if t.DefaultMs <= 0 {
		return false
	}
	t.RemainingMs = t.DefaultMs
	t.UpdatedAt = now
	t.Running = false
	return true
}

func (s *Store) TimerSet(now time.Time, remainingMs int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	t := &s.match.Timer
	if t.DefaultMs <= 0 {
		return false
	}
	if remainingMs < 0 {
		remainingMs = 0
	}
	// Hard cap to 9999:59 to prevent unreasonable input.
	const maxMs = int64((9999*60 + 59) * 1000)
	if remainingMs > maxMs {
		remainingMs = maxMs
	}
	// Setting time also updates the per-match default (for "Reset" + UI).
	t.DefaultMs = remainingMs
	t.RemainingMs = remainingMs
	if remainingMs == 0 {
		t.Running = false
	}
	t.UpdatedAt = now
	return true
}
