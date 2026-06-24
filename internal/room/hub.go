package room

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const DefaultRoomIdleTimeout = 30 * time.Minute

type Hub struct {
	mu          sync.Mutex
	rooms       map[string]*Actor
	idleTimeout time.Duration
}

type HubOption func(*Hub)

func WithRoomIdleTimeout(timeout time.Duration) HubOption {
	return func(h *Hub) {
		h.idleTimeout = timeout
	}
}

func NewHub(options ...HubOption) *Hub {
	hub := &Hub{
		rooms:       make(map[string]*Actor),
		idleTimeout: DefaultRoomIdleTimeout,
	}
	for _, option := range options {
		option(hub)
	}
	return hub
}

func (h *Hub) GetOrCreate(roomID string) *Actor {
	h.mu.Lock()
	defer h.mu.Unlock()

	if actor, ok := h.rooms[roomID]; ok {
		if !actor.Closed() {
			return actor
		}
		delete(h.rooms, roomID)
	}

	var actor *Actor
	actor = newActor(roomID, h.idleTimeout, func() {
		h.remove(roomID, actor)
	})
	h.rooms[roomID] = actor
	return actor
}

func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}

func (h *Hub) remove(roomID string, actor *Actor) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[roomID] == actor {
		delete(h.rooms, roomID)
	}
}

func randomSubscriptionID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}
