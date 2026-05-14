package match

import (
	"sync"
	"time"
)

type EditMatch struct {
	MatchID int64

	Team1 TeamSide
	Team2 TeamSide

	StartedAt time.Time
	EndedAt   time.Time

	GoalsByPlayerID map[string]int
}

type EditStore struct {
	mu   sync.RWMutex
	edit *EditMatch
}

func NewEditStore() *EditStore {
	return &EditStore{}
}

func (s *EditStore) Get() *EditMatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.edit == nil {
		return nil
	}
	cp := *s.edit
	cp.GoalsByPlayerID = make(map[string]int, len(s.edit.GoalsByPlayerID))
	for k, v := range s.edit.GoalsByPlayerID {
		cp.GoalsByPlayerID[k] = v
	}
	return &cp
}

func (s *EditStore) Start(e *EditMatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edit = e
}

func (s *EditStore) Discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edit = nil
}

func (s *EditStore) Inc(matchID int64, playerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.edit == nil || s.edit.MatchID != matchID {
		return false
	}
	s.edit.GoalsByPlayerID[playerID]++
	return true
}

func (s *EditStore) Dec(matchID int64, playerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.edit == nil || s.edit.MatchID != matchID {
		return false
	}
	if s.edit.GoalsByPlayerID[playerID] <= 0 {
		s.edit.GoalsByPlayerID[playerID] = 0
		return true
	}
	s.edit.GoalsByPlayerID[playerID]--
	return true
}

