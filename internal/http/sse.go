package http

import (
	"sync"
)

type ScoreHub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

func NewScoreHub() *ScoreHub {
	return &ScoreHub{subs: map[chan []byte]struct{}{}}
}

func (h *ScoreHub) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *ScoreHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *ScoreHub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}
