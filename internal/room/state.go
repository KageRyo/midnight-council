package room

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	MinPlayersToStart = 2
	MaxChatBytes      = 500
)

var (
	ErrPlayerIDRequired   = errors.New("player id is required")
	ErrPlayerNameRequired = errors.New("player name is required")
	ErrPlayerNotFound     = errors.New("player not found")
	ErrOwnerOnly          = errors.New("only the room owner can perform this action")
	ErrRoomNotWaiting     = errors.New("room is not waiting")
	ErrNotEnoughPlayers   = errors.New("not enough players to start")
	ErrPlayersNotReady    = errors.New("all non-owner players must be ready")
	ErrEmptyMessage       = errors.New("message is empty")
	ErrMessageTooLong     = errors.New("message is too long")
)

type State struct {
	roomID    string
	phase     Phase
	ownerID   string
	players   map[string]*Player
	updatedAt time.Time
}

func NewState(roomID string) *State {
	now := time.Now().UTC()
	return &State{
		roomID:    roomID,
		phase:     PhaseWaiting,
		players:   make(map[string]*Player),
		updatedAt: now,
	}
}

func (s *State) Apply(event Event) (*Envelope, error) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	switch event.Type {
	case EventJoin:
		return s.applyJoin(event)
	case EventLeave:
		return s.applyLeave(event)
	case EventReady:
		return s.applyReady(event)
	case EventStartGame:
		return s.applyStartGame(event)
	case EventChat:
		return s.applyChat(event)
	default:
		return nil, fmt.Errorf("unknown room event: %s", event.Type)
	}
}

func (s *State) Snapshot() Snapshot {
	players := make([]PlayerView, 0, len(s.players))
	for _, player := range s.players {
		players = append(players, PlayerView{
			ID:        player.ID,
			Name:      player.Name,
			Ready:     player.Ready,
			Connected: player.Connected,
			Owner:     player.ID == s.ownerID,
		})
	}

	sort.Slice(players, func(i, j int) bool {
		return players[i].Name < players[j].Name
	})

	return Snapshot{
		RoomID:    s.roomID,
		OwnerID:   s.ownerID,
		Phase:     s.phase,
		Players:   players,
		UpdatedAt: s.updatedAt,
	}
}

func (s *State) applyJoin(event Event) (*Envelope, error) {
	if strings.TrimSpace(event.PlayerID) == "" {
		return nil, ErrPlayerIDRequired
	}
	if strings.TrimSpace(event.PlayerName) == "" {
		return nil, ErrPlayerNameRequired
	}

	player, exists := s.players[event.PlayerID]
	if exists {
		player.Name = strings.TrimSpace(event.PlayerName)
		player.Connected = true
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	s.players[event.PlayerID] = &Player{
		ID:        event.PlayerID,
		Name:      strings.TrimSpace(event.PlayerName),
		Connected: true,
		JoinedAt:  event.At,
	}
	if s.ownerID == "" {
		s.ownerID = event.PlayerID
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyLeave(event Event) (*Envelope, error) {
	player, ok := s.players[event.PlayerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}

	if s.phase == PhaseWaiting {
		delete(s.players, event.PlayerID)
		if s.ownerID == event.PlayerID {
			s.ownerID = s.nextOwnerID()
		}
	} else {
		player.Connected = false
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyReady(event Event) (*Envelope, error) {
	if s.phase != PhaseWaiting {
		return nil, ErrRoomNotWaiting
	}
	player, ok := s.players[event.PlayerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}

	player.Ready = event.Ready
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyStartGame(event Event) (*Envelope, error) {
	if s.phase != PhaseWaiting {
		return nil, ErrRoomNotWaiting
	}
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	if len(s.players) < MinPlayersToStart {
		return nil, ErrNotEnoughPlayers
	}
	if !s.nonOwnersReady() {
		return nil, ErrPlayersNotReady
	}

	s.phase = PhaseRoleAssignment
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyChat(event Event) (*Envelope, error) {
	player, ok := s.players[event.PlayerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}

	message := strings.TrimSpace(event.Message)
	if message == "" {
		return nil, ErrEmptyMessage
	}
	if len([]byte(message)) > MaxChatBytes {
		return nil, ErrMessageTooLong
	}

	s.touch(event.At)
	return &Envelope{
		Type: "chat",
		Chat: &ChatMessage{
			RoomID:   s.roomID,
			PlayerID: player.ID,
			Name:     player.Name,
			Message:  message,
			SentAt:   event.At,
		},
	}, nil
}

func (s *State) nonOwnersReady() bool {
	for _, player := range s.players {
		if player.ID != s.ownerID && !player.Ready {
			return false
		}
	}
	return true
}

func (s *State) nextOwnerID() string {
	ids := make([]string, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (s *State) touch(at time.Time) {
	s.updatedAt = at.UTC()
}

func stateEnvelope(snapshot Snapshot) *Envelope {
	return &Envelope{
		Type:  "state",
		State: &snapshot,
	}
}
