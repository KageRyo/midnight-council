package room

import "time"

type Phase string

const (
	PhaseWaiting       Phase = "WAITING"
	PhaseNight         Phase = "NIGHT"
	PhaseDayDiscussion Phase = "DAY_DISCUSSION"
	PhaseDayVoting     Phase = "DAY_VOTING"
	PhaseFinished      Phase = "FINISHED"
)

type EventType string

const (
	EventJoin         EventType = "join"
	EventLeave        EventType = "leave"
	EventReady        EventType = "ready"
	EventStartGame    EventType = "start_game"
	EventChat         EventType = "chat"
	EventNightAction  EventType = "night_action"
	EventNightPass    EventType = "night_pass"
	EventStartVote    EventType = "start_vote"
	EventVote         EventType = "vote"
	EventShoot        EventType = "shoot"
	EventPhaseTimeout EventType = "phase_timeout"
)

type Role string

const (
	RoleVillager  Role = "VILLAGER"
	RoleKiller    Role = "KILLER"
	RoleDetective Role = "DETECTIVE"
	RoleDoctor    Role = "DOCTOR"
	RoleShooter   Role = "SHOOTER"
)

type Winner string

const (
	WinnerVillagers Winner = "VILLAGERS"
	WinnerKillers   Winner = "KILLERS"
)

type NightActionType string

const (
	NightActionKill        NightActionType = "KILL"
	NightActionInvestigate NightActionType = "INVESTIGATE"
	NightActionProtect     NightActionType = "PROTECT"
	NightActionPass        NightActionType = "PASS"
)

type LogType string

const (
	LogGameStarted        LogType = "game_started"
	LogNightStarted       LogType = "night_started"
	LogDayStarted         LogType = "day_started"
	LogNightEliminated    LogType = "night_eliminated"
	LogNightNoElimination LogType = "night_no_elimination"
	LogVotingStarted      LogType = "voting_started"
	LogPlayerExecuted     LogType = "player_executed"
	LogVoteNoExecution    LogType = "vote_no_execution"
	LogShooterFired       LogType = "shooter_fired"
	LogPhaseTimedOut      LogType = "phase_timed_out"
	LogGameFinished       LogType = "game_finished"
)

type Event struct {
	Type           EventType
	PlayerID       string
	PlayerName     string
	ReconnectToken string
	Ready          bool
	Message        string
	TargetID       string
	At             time.Time
}

type Player struct {
	ID             string
	Name           string
	ReconnectToken string
	Ready          bool
	Connected      bool
	Role           Role
	Alive          bool
	ShooterUsed    bool
	JoinedAt       time.Time
}

type PlayerView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
	Owner     bool   `json:"owner"`
	Alive     bool   `json:"alive"`
	Role      Role   `json:"role,omitempty"`
}

type Snapshot struct {
	RoomID         string       `json:"room_id"`
	OwnerID        string       `json:"owner_id"`
	Phase          Phase        `json:"phase"`
	PhaseStartedAt time.Time    `json:"phase_started_at"`
	PhaseDeadline  *time.Time   `json:"phase_deadline,omitempty"`
	Round          int          `json:"round"`
	Players        []PlayerView `json:"players"`
	Result         *GameResult  `json:"result,omitempty"`
	Log            []LogEntry   `json:"log,omitempty"`
	UpdatedAt      time.Time    `json:"updated_at"`
	ServerTime     time.Time    `json:"server_time"`
}

type PrivatePlayerView struct {
	PlayerID       string                `json:"player_id"`
	ReconnectToken string                `json:"reconnect_token,omitempty"`
	Role           Role                  `json:"role,omitempty"`
	Alive          bool                  `json:"alive"`
	ActionRequired bool                  `json:"action_required"`
	Available      []EventType           `json:"available,omitempty"`
	CanVote        bool                  `json:"can_vote"`
	VotedFor       string                `json:"voted_for,omitempty"`
	CanShoot       bool                  `json:"can_shoot"`
	Investigations []InvestigationResult `json:"investigations,omitempty"`
}

type InvestigationResult struct {
	Round      int    `json:"round"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name"`
	Killer     bool   `json:"killer"`
}

type NightAction struct {
	PlayerID string
	TargetID string
	Type     NightActionType
}

type GameResult struct {
	Winner     Winner    `json:"winner"`
	Reason     string    `json:"reason"`
	FinishedAt time.Time `json:"finished_at"`
}

type LogEntry struct {
	Type     LogType   `json:"type"`
	Round    int       `json:"round"`
	Phase    Phase     `json:"phase,omitempty"`
	PlayerID string    `json:"player_id,omitempty"`
	TargetID string    `json:"target_id,omitempty"`
	Winner   Winner    `json:"winner,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	At       time.Time `json:"at"`
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
