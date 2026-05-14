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
