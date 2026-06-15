package room

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Actor
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]*Actor),
	}
}

func (h *Hub) GetOrCreate(roomID string) *Actor {
	h.mu.Lock()
	defer h.mu.Unlock()

	if actor, ok := h.rooms[roomID]; ok {
		return actor
	}

	actor := NewActor(roomID)
	h.rooms[roomID] = actor
	return actor
}

func randomSubscriptionID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}
