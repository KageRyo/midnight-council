package room

import "time"

type Phase string

const (
	PhaseWaiting        Phase = "WAITING"
	PhaseRoleAssignment Phase = "ROLE_ASSIGNMENT"
)

type EventType string

const (
	EventJoin      EventType = "join"
	EventLeave     EventType = "leave"
	EventReady     EventType = "ready"
	EventStartGame EventType = "start_game"
	EventChat      EventType = "chat"
)

type Event struct {
	Type       EventType
	PlayerID   string
	PlayerName string
	Ready      bool
	Message    string
	At         time.Time
}

type Player struct {
	ID        string
	Name      string
	Ready     bool
	Connected bool
	JoinedAt  time.Time
}

type PlayerView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
	Owner     bool   `json:"owner"`
}

type Snapshot struct {
	RoomID    string       `json:"room_id"`
	OwnerID   string       `json:"owner_id"`
	Phase     Phase        `json:"phase"`
	Players   []PlayerView `json:"players"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type ChatMessage struct {
	RoomID   string    `json:"room_id"`
	PlayerID string    `json:"player_id"`
	Name     string    `json:"name"`
	Message  string    `json:"message"`
	SentAt   time.Time `json:"sent_at"`
}

type Envelope struct {
	Type    string       `json:"type"`
	State   *Snapshot    `json:"state,omitempty"`
	Chat    *ChatMessage `json:"chat,omitempty"`
	Error   string       `json:"error,omitempty"`
	Private any          `json:"private,omitempty"`
}
