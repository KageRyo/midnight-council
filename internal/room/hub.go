package room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const DefaultRoomIdleTimeout = 30 * time.Minute

const DefaultMaxRooms = 1000

var ErrRoomLimitReached = errors.New("server room limit has been reached")

type Hub struct {
	mu             sync.Mutex
	rooms          map[string]*Actor
	idleTimeout    time.Duration
	phaseDurations PhaseDurations
	ruleSet        *RuleSet
	maxRooms       int
	maxSpectators  int
}

type HubOption func(*Hub)

func WithRoomIdleTimeout(timeout time.Duration) HubOption {
	return func(h *Hub) {
		h.idleTimeout = timeout
	}
}

func WithPhaseDurations(durations PhaseDurations) HubOption {
	return func(h *Hub) {
		h.phaseDurations = durations
	}
}

func WithRuleSet(rules *RuleSet) HubOption {
	return func(h *Hub) {
		h.ruleSet = rules
	}
}

func WithMaxRooms(maxRooms int) HubOption {
	return func(h *Hub) {
		h.maxRooms = maxRooms
	}
}

func WithMaxSpectators(maxSpectators int) HubOption {
	return func(h *Hub) {
		h.maxSpectators = maxSpectators
	}
}

func NewHub(options ...HubOption) *Hub {
	hub := &Hub{
		rooms:          make(map[string]*Actor),
		idleTimeout:    DefaultRoomIdleTimeout,
		phaseDurations: DefaultPhaseDurations(),
		ruleSet:        DefaultRuleSet(),
		maxRooms:       DefaultMaxRooms,
		maxSpectators:  DefaultMaxSpectators,
	}
	for _, option := range options {
		option(hub)
	}
	if err := hub.phaseDurations.Validate(); err != nil {
		panic(err)
	}
	if hub.ruleSet == nil {
		panic(ErrInvalidRuleSet)
	}
	if hub.maxRooms <= 0 {
		panic("max rooms must be positive")
	}
	if hub.maxSpectators <= 0 {
		panic("max spectators must be positive")
	}
	return hub
}

func (h *Hub) GetOrCreate(roomID string) (*Actor, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if actor, ok := h.rooms[roomID]; ok {
		if !actor.Closed() {
			return actor, nil
		}
		delete(h.rooms, roomID)
	}
	if len(h.rooms) >= h.maxRooms {
		return nil, ErrRoomLimitReached
	}

	actor := newActorWithRuleSet(roomID, h.idleTimeout, h.phaseDurations, h.ruleSet, h.maxSpectators, func(actor *Actor) {
		h.remove(roomID, actor)
	})
	h.rooms[roomID] = actor
	return actor, nil
}

func (h *Hub) RoomExists(roomID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	actor, ok := h.rooms[roomID]
	return ok && !actor.Closed()
}

func (h *Hub) CanCreateRoom() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms) < h.maxRooms
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
