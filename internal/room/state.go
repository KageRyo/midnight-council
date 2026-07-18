package room

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

const (
	MinPlayersToStart  = 2
	DefaultMaxPlayers  = 12
	MaxPlayersAllowed  = 20
	MaxChatBytes       = 500
	MaxEventLogEntries = 100
)

var (
	ErrPlayerIDRequired        = errors.New("player id is required")
	ErrPlayerNameRequired      = errors.New("player name is required")
	ErrPlayerNotFound          = errors.New("player not found")
	ErrReconnectTokenRequired  = errors.New("reconnect token is required")
	ErrInvalidReconnectToken   = errors.New("reconnect token is invalid")
	ErrOwnerOnly               = errors.New("only the room owner can perform this action")
	ErrRoomNotWaiting          = errors.New("room is not waiting")
	ErrRoomNotJoinable         = errors.New("room is not joinable")
	ErrRoomLocked              = errors.New("room is locked")
	ErrRoomFull                = errors.New("room player limit has been reached")
	ErrWrongPhase              = errors.New("action is not allowed in the current phase")
	ErrGameFinished            = errors.New("game is already finished")
	ErrNotEnoughPlayers        = errors.New("not enough players to start")
	ErrPlayersNotReady         = errors.New("all non-owner players must be ready")
	ErrEmptyMessage            = errors.New("message is empty")
	ErrMessageTooLong          = errors.New("message is too long")
	ErrPlayerDead              = errors.New("player is dead")
	ErrInvalidTarget           = errors.New("target is invalid")
	ErrSelfTargetNotAllowed    = errors.New("self target is not allowed")
	ErrRoleHasNoNightAction    = errors.New("role has no night action")
	ErrShooterOnly             = errors.New("only the shooter can perform this action")
	ErrShooterAlreadyUsed      = errors.New("shooter action has already been used")
	ErrPhaseDeadlineNotReached = errors.New("phase deadline has not been reached")
	ErrParticipantTypeMismatch = errors.New("participant type does not match the existing seat")
	ErrSpectatorCannotChat     = errors.New("spectators cannot chat during an active game")
	ErrLastWordsSpeakerOnly    = errors.New("only the executed player can speak during last words")
	ErrParticipantNotFound     = errors.New("participant not found")
	ErrCannotKickOwner         = errors.New("the room owner cannot be kicked")
	ErrKickNotAllowed          = errors.New("participants can only be kicked while waiting or after a game")
	ErrKicked                  = errors.New("removed from room by the owner")
	ErrInvalidOwnerTarget      = errors.New("new owner must be a connected seated player")
	ErrPlayerLimitOutOfRange   = errors.New("player limit is out of range")
	ErrPlayerLimitBelowCurrent = errors.New("player limit cannot be below the current player count")
	ErrAlreadyWaiting          = errors.New("room is already waiting")
	ErrConnectionReplaced      = errors.New("connection replaced by a newer session")
	ErrDuplicateClientEvent    = errors.New("client event was already processed")
)

type State struct {
	roomID         string
	phase          Phase
	phaseStartedAt time.Time
	phaseDeadline  time.Time
	baseDurations  PhaseDurations
	phaseDurations PhaseDurations
	rules          *RuleSet
	gameSettings   GameSettings
	ownerID        string
	players        map[string]*Player
	spectators     map[string]*Spectator
	locked         bool
	maxPlayers     int
	round          int
	nightActions   map[string]NightAction
	votes          map[string]string
	investigations map[string][]InvestigationResult
	lastWordsID    string
	result         *GameResult
	log            []LogEntry
	updatedAt      time.Time
}

func NewState(roomID string) *State {
	return newState(roomID, DefaultPhaseDurations())
}

func NewStateWithRuleSet(roomID string, rules *RuleSet) *State {
	return newStateWithRuleSet(roomID, DefaultPhaseDurations(), rules)
}

func newState(roomID string, phaseDurations PhaseDurations) *State {
	return newStateWithRuleSet(roomID, phaseDurations, DefaultRuleSet())
}

func newStateWithRuleSet(roomID string, phaseDurations PhaseDurations, rules *RuleSet) *State {
	if err := phaseDurations.Validate(); err != nil {
		panic(err)
	}
	if rules == nil {
		panic(ErrInvalidRuleSet)
	}
	now := time.Now().UTC()
	return &State{
		roomID:         roomID,
		phase:          PhaseWaiting,
		phaseStartedAt: now,
		baseDurations:  phaseDurations,
		phaseDurations: phaseDurations,
		rules:          rules,
		gameSettings:   StandardGameSettings(phaseDurations),
		players:        make(map[string]*Player),
		spectators:     make(map[string]*Spectator),
		maxPlayers:     DefaultMaxPlayers,
		nightActions:   make(map[string]NightAction),
		votes:          make(map[string]string),
		investigations: make(map[string][]InvestigationResult),
		updatedAt:      now,
	}
}

func (s *State) Apply(event Event) (*Envelope, error) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}

	if event.ClientSequence > 0 {
		if last, ok := s.lastClientSequence(event.PlayerID); ok && event.ClientSequence <= last {
			return nil, fmt.Errorf("%w: sequence %d", ErrDuplicateClientEvent, event.ClientSequence)
		}
	}

	var envelope *Envelope
	var err error
	switch event.Type {
	case EventJoin:
		envelope, err = s.applyJoin(event)
	case EventLeave:
		envelope, err = s.applyLeave(event)
	case EventDisconnect:
		envelope, err = s.applyDisconnect(event)
	case EventReady:
		envelope, err = s.applyReady(event)
	case EventStartGame:
		envelope, err = s.applyStartGame(event)
	case EventChat:
		envelope, err = s.applyChat(event)
	case EventNightAction:
		envelope, err = s.applyNightAction(event)
	case EventNightPass:
		envelope, err = s.applyNightPass(event)
	case EventStartVote:
		envelope, err = s.applyStartVote(event)
	case EventVote:
		envelope, err = s.applyVote(event)
	case EventShoot:
		envelope, err = s.applyShoot(event)
	case EventTransferOwner:
		envelope, err = s.applyTransferOwner(event)
	case EventKickParticipant:
		envelope, err = s.applyKickParticipant(event)
	case EventSetRoomLocked:
		envelope, err = s.applySetRoomLocked(event)
	case EventSetPlayerLimit:
		envelope, err = s.applySetPlayerLimit(event)
	case EventSetGameSettings:
		envelope, err = s.applySetGameSettings(event)
	case EventSetGamePreset:
		envelope, err = s.applySetGamePreset(event)
	case EventPresence:
		envelope, err = s.applyPresence(event)
	case EventReturnToWaiting:
		envelope, err = s.applyReturnToWaiting(event)
	case EventPhaseTimeout:
		envelope, err = s.applyPhaseTimeout(event)
	default:
		return nil, fmt.Errorf("unknown room event: %s", event.Type)
	}
	if err != nil {
		return nil, err
	}
	if event.ClientSequence > 0 {
		s.recordClientSequence(event.PlayerID, event.ClientSequence)
	}
	return envelope, nil
}

func (s *State) Snapshot() Snapshot {
	players := make([]PlayerView, 0, len(s.players))

	for _, player := range s.players {
		alive := player.Alive
		if s.phase == PhaseWaiting {
			alive = true
		}

		view := PlayerView{
			ID:        player.ID,
			Name:      player.Name,
			Ready:     player.Ready,
			Connected: player.Connected,
			Owner:     player.ID == s.ownerID,
			Alive:     alive,
			AFK:       player.AFK,
		}
		if s.phase == PhaseFinished ||
			(s.gameSettings.RevealRolesOnDeath && s.phase != PhaseWaiting && !player.Alive) {
			view.Role = player.Role
		}
		players = append(players, view)
	}

	sort.Slice(players, func(i, j int) bool {
		if players[i].Name == players[j].Name {
			return players[i].ID < players[j].ID
		}
		return players[i].Name < players[j].Name
	})

	spectators := make([]SpectatorView, 0, len(s.spectators))
	for _, spectator := range s.spectators {
		spectators = append(spectators, SpectatorView{
			ID:        spectator.ID,
			Name:      spectator.Name,
			Connected: spectator.Connected,
			AFK:       spectator.AFK,
		})
	}
	sort.Slice(spectators, func(i, j int) bool {
		if spectators[i].Name == spectators[j].Name {
			return spectators[i].ID < spectators[j].ID
		}
		return spectators[i].Name < spectators[j].Name
	})

	entries := make([]LogEntry, len(s.log))
	copy(entries, s.log)
	var phaseDeadline *time.Time
	if !s.phaseDeadline.IsZero() {
		deadline := s.phaseDeadline
		phaseDeadline = &deadline
	}

	return Snapshot{
		RoomID:         s.roomID,
		OwnerID:        s.ownerID,
		Phase:          s.phase,
		PhaseStartedAt: s.phaseStartedAt,
		PhaseDeadline:  phaseDeadline,
		Round:          s.round,
		Players:        players,
		Spectators:     spectators,
		Locked:         s.locked,
		MaxPlayers:     s.maxPlayers,
		GameSettings:   s.gameSettings,
		GamePresets:    GamePresetCatalog(s.baseDurations),
		LastWordsID:    s.lastWordsID,
		Result:         s.result,
		Log:            entries,
		UpdatedAt:      s.updatedAt,
		ServerTime:     time.Now().UTC(),
	}
}

func (s *State) PrivateView(playerID string) *PrivatePlayerView {
	player, ok := s.players[playerID]
	if !ok {
		spectator, spectatorOK := s.spectators[playerID]
		if !spectatorOK {
			return nil
		}
		return &PrivatePlayerView{
			PlayerID:       spectator.ID,
			ReconnectToken: spectator.ReconnectToken,
			Spectator:      true,
		}
	}

	alive := player.Alive
	if s.phase == PhaseWaiting {
		alive = true
	}

	view := &PrivatePlayerView{
		PlayerID:       player.ID,
		ReconnectToken: player.ReconnectToken,
		Alive:          alive,
		Investigations: append([]InvestigationResult(nil), s.investigations[player.ID]...),
	}

	if s.phase != PhaseWaiting {
		view.Role = player.Role
	}

	if _, hasNightAction := s.gameRules().NightActionForRole(player.Role); s.phase == PhaseNight && player.Alive && hasNightAction {
		view.ActionRequired = !s.nightActionSubmitted(player.ID)
		view.Available = append(view.Available, EventNightAction, EventNightPass)
	}

	if s.phase == PhaseDayVoting && player.Alive {
		view.CanVote = true
		view.VotedFor = s.votes[player.ID]
		view.Available = append(view.Available, EventVote)
	}

	if isDayPhase(s.phase) && player.Alive && s.gameRules().CanShoot(player.Role) && !player.ShooterUsed {
		view.CanShoot = true
		view.Available = append(view.Available, EventShoot)
	}

	return view
}

func (s *State) EnvelopeForPlayer(playerID string) *Envelope {
	snapshot := s.Snapshot()
	return &Envelope{
		Type:    "state",
		State:   &snapshot,
		Private: s.PrivateView(playerID),
	}
}

func (s *State) Personalize(envelope Envelope, playerID string) Envelope {
	if envelope.Type != "state" {
		return envelope
	}
	if envelope.State == nil {
		snapshot := s.Snapshot()
		envelope.State = &snapshot
	}
	envelope.Private = s.PrivateView(playerID)
	return envelope
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
		if event.Spectator {
			return nil, ErrParticipantTypeMismatch
		}
		if strings.TrimSpace(event.ReconnectToken) == "" {
			return nil, ErrReconnectTokenRequired
		}
		if event.ReconnectToken != player.ReconnectToken {
			return nil, ErrInvalidReconnectToken
		}
		player.Name = strings.TrimSpace(event.PlayerName)
		player.Connected = true
		player.AFK = false
		s.ensureConnectedOwner()
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	spectator, exists := s.spectators[event.PlayerID]
	if exists {
		if !event.Spectator {
			return nil, ErrParticipantTypeMismatch
		}
		if strings.TrimSpace(event.ReconnectToken) == "" {
			return nil, ErrReconnectTokenRequired
		}
		if event.ReconnectToken != spectator.ReconnectToken {
			return nil, ErrInvalidReconnectToken
		}
		spectator.Name = strings.TrimSpace(event.PlayerName)
		spectator.Connected = true
		spectator.AFK = false
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	if s.locked {
		return nil, ErrRoomLocked
	}

	if event.Spectator {
		reconnectToken, err := newReconnectToken()
		if err != nil {
			return nil, err
		}
		s.spectators[event.PlayerID] = &Spectator{
			ID:             event.PlayerID,
			Name:           strings.TrimSpace(event.PlayerName),
			ReconnectToken: reconnectToken,
			Connected:      true,
			JoinedAt:       event.At,
		}
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	if s.phase != PhaseWaiting {
		return nil, ErrRoomNotJoinable
	}
	if len(s.players) >= s.maxPlayers {
		return nil, ErrRoomFull
	}

	reconnectToken, err := newReconnectToken()
	if err != nil {
		return nil, err
	}
	s.players[event.PlayerID] = &Player{
		ID:             event.PlayerID,
		Name:           strings.TrimSpace(event.PlayerName),
		ReconnectToken: reconnectToken,
		Connected:      true,
		Alive:          true,
		JoinedAt:       event.At,
	}
	if s.ownerID == "" {
		s.ownerID = event.PlayerID
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyLeave(event Event) (*Envelope, error) {
	player, ok := s.players[event.PlayerID]
	if ok {
		if s.phase == PhaseWaiting {
			delete(s.players, event.PlayerID)
			if s.ownerID == event.PlayerID {
				s.ownerID = s.nextOwnerID()
			}
		} else {
			player.Connected = false
			player.AFK = false
			if s.ownerID == event.PlayerID {
				s.ensureConnectedOwner()
			}
		}

		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	spectator, ok := s.spectators[event.PlayerID]
	if !ok {
		return nil, ErrParticipantNotFound
	}
	if s.phase == PhaseWaiting {
		delete(s.spectators, event.PlayerID)
	} else {
		spectator.Connected = false
		spectator.AFK = false
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyDisconnect(event Event) (*Envelope, error) {
	if player, ok := s.players[event.PlayerID]; ok {
		player.Connected = false
		player.AFK = false
		if s.ownerID == event.PlayerID {
			s.ensureConnectedOwner()
		}
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	if spectator, ok := s.spectators[event.PlayerID]; ok {
		spectator.Connected = false
		spectator.AFK = false
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}
	return nil, ErrParticipantNotFound
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

func (s *State) applyPresence(event Event) (*Envelope, error) {
	if player, ok := s.players[event.PlayerID]; ok {
		player.AFK = event.AFK
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}
	if spectator, ok := s.spectators[event.PlayerID]; ok {
		spectator.AFK = event.AFK
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}
	return nil, ErrParticipantNotFound
}

func (s *State) applyStartGame(event Event) (*Envelope, error) {
	if s.phase == PhaseFinished {
		return nil, ErrGameFinished
	}
	if s.phase != PhaseWaiting {
		return nil, ErrRoomNotWaiting
	}
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	durations, err := s.gameSettings.ValidateForStartWithRuleSet(s.maxPlayers, s.baseDurations, s.gameRules())
	if err != nil {
		return nil, err
	}
	if len(s.players) < s.gameSettings.MinimumPlayers {
		return nil, ErrNotEnoughPlayers
	}
	if !s.nonOwnersReady() {
		return nil, ErrPlayersNotReady
	}

	roles := s.gameRules().RoleDeck(len(s.players), s.gameSettings.Roles)
	if err := shuffleRoles(roles); err != nil {
		return nil, err
	}
	s.phaseDurations = durations

	ids := s.sortedPlayerIDs()
	for i, id := range ids {
		player := s.players[id]
		player.Role = roles[i]
		player.Alive = true
		player.ShooterUsed = false
	}

	s.enterPhase(PhaseNight, event.At)
	s.round = 1
	s.nightActions = make(map[string]NightAction)
	s.votes = make(map[string]string)
	s.investigations = make(map[string][]InvestigationResult)
	s.lastWordsID = ""
	s.result = nil
	s.log = nil
	s.appendLog(LogGameStarted, event.At, LogEntry{PlayerID: event.PlayerID})
	s.appendLog(LogNightStarted, event.At, LogEntry{})
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyChat(event Event) (*Envelope, error) {
	player, ok := s.players[event.PlayerID]
	name := ""
	if ok {
		if s.phase == PhaseLastWords && player.ID != s.lastWordsID {
			return nil, ErrLastWordsSpeakerOnly
		}
		if s.phase != PhaseWaiting && s.phase != PhaseFinished && s.phase != PhaseLastWords && !player.Alive {
			return nil, ErrPlayerDead
		}
		name = player.Name
	} else {
		spectator, spectatorOK := s.spectators[event.PlayerID]
		if !spectatorOK {
			return nil, ErrParticipantNotFound
		}
		if s.phase != PhaseWaiting && s.phase != PhaseFinished {
			return nil, ErrSpectatorCannotChat
		}
		name = spectator.Name
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
			PlayerID: event.PlayerID,
			Name:     name,
			Message:  message,
			SentAt:   event.At,
		},
	}, nil
}

func (s *State) applyNightAction(event Event) (*Envelope, error) {
	if s.phase != PhaseNight {
		return nil, ErrWrongPhase
	}

	player, err := s.livingPlayer(event.PlayerID)
	if err != nil {
		return nil, err
	}

	actionRule, ok := s.gameRules().NightActionForRole(player.Role)
	if !ok {
		return nil, ErrRoleHasNoNightAction
	}

	target, err := s.livingTarget(event.TargetID)
	if err != nil {
		return nil, err
	}

	if target.ID == player.ID && !actionRule.AllowSelfTarget {
		return nil, ErrSelfTargetNotAllowed
	}

	s.nightActions[player.ID] = NightAction{
		PlayerID: player.ID,
		TargetID: target.ID,
		Type:     actionRule.Type,
	}

	if s.allNightActionsSubmitted() {
		s.resolveNight(event.At)
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyNightPass(event Event) (*Envelope, error) {
	if s.phase != PhaseNight {
		return nil, ErrWrongPhase
	}

	player, err := s.livingPlayer(event.PlayerID)
	if err != nil {
		return nil, err
	}
	if _, ok := s.gameRules().NightActionForRole(player.Role); !ok {
		return nil, ErrRoleHasNoNightAction
	}

	s.nightActions[player.ID] = NightAction{
		PlayerID: player.ID,
		Type:     NightActionPass,
	}

	if s.allNightActionsSubmitted() {
		s.resolveNight(event.At)
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyStartVote(event Event) (*Envelope, error) {
	if s.phase != PhaseDayDiscussion {
		return nil, ErrWrongPhase
	}
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}

	s.startVoting(event.At, event.PlayerID)
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyVote(event Event) (*Envelope, error) {
	if s.phase != PhaseDayVoting {
		return nil, ErrWrongPhase
	}

	voter, err := s.livingPlayer(event.PlayerID)
	if err != nil {
		return nil, err
	}

	targetID := strings.TrimSpace(event.TargetID)
	if targetID != "" {
		if _, err := s.livingTarget(targetID); err != nil {
			return nil, err
		}
	}

	s.votes[voter.ID] = targetID
	if s.allLivingPlayersVoted() {
		s.resolveVote(event.At)
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyShoot(event Event) (*Envelope, error) {
	if !isDayPhase(s.phase) {
		return nil, ErrWrongPhase
	}

	shooter, err := s.livingPlayer(event.PlayerID)
	if err != nil {
		return nil, err
	}
	if !s.gameRules().CanShoot(shooter.Role) {
		return nil, ErrShooterOnly
	}
	if shooter.ShooterUsed {
		return nil, ErrShooterAlreadyUsed
	}

	target, err := s.livingTarget(event.TargetID)
	if err != nil {
		return nil, err
	}
	if target.ID == shooter.ID {
		return nil, ErrSelfTargetNotAllowed
	}

	shooter.ShooterUsed = true
	target.Alive = false
	s.appendLog(LogShooterFired, event.At, LogEntry{
		PlayerID: shooter.ID,
		TargetID: target.ID,
	})
	s.pruneVotes()

	if !s.checkWin(event.At) && s.phase == PhaseDayVoting && s.allLivingPlayersVoted() {
		s.resolveVote(event.At)
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyTransferOwner(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}

	targetID := strings.TrimSpace(event.TargetID)
	target, ok := s.players[targetID]
	if !ok || !target.Connected {
		return nil, ErrInvalidOwnerTarget
	}

	s.ownerID = target.ID
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyKickParticipant(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	if s.phase != PhaseWaiting && s.phase != PhaseFinished {
		return nil, ErrKickNotAllowed
	}

	targetID := strings.TrimSpace(event.TargetID)
	if targetID == s.ownerID {
		return nil, ErrCannotKickOwner
	}
	if _, ok := s.players[targetID]; ok {
		delete(s.players, targetID)
		delete(s.nightActions, targetID)
		delete(s.votes, targetID)
		delete(s.investigations, targetID)
		for voterID, votedFor := range s.votes {
			if votedFor == targetID {
				s.votes[voterID] = ""
			}
		}
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}
	if _, ok := s.spectators[targetID]; ok {
		delete(s.spectators, targetID)
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}
	return nil, ErrParticipantNotFound
}

func (s *State) applySetRoomLocked(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}

	s.locked = event.Locked
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applySetPlayerLimit(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	if s.phase != PhaseWaiting && s.phase != PhaseFinished {
		return nil, ErrWrongPhase
	}
	if event.MaxPlayers < MinPlayersToStart || event.MaxPlayers > MaxPlayersAllowed {
		return nil, ErrPlayerLimitOutOfRange
	}
	if event.MaxPlayers < len(s.players) {
		return nil, ErrPlayerLimitBelowCurrent
	}
	if event.MaxPlayers < s.gameSettings.MinimumPlayers {
		return nil, ErrPlayerLimitBelowMinimum
	}

	s.maxPlayers = event.MaxPlayers
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applySetGameSettings(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	if s.phase != PhaseWaiting && s.phase != PhaseFinished {
		return nil, ErrWrongPhase
	}

	settings := event.GameSettings
	settings.Preset = GamePresetCustom
	durations, err := settings.ValidateWithRuleSet(s.maxPlayers, s.gameRules())
	if err != nil {
		return nil, err
	}

	s.gameSettings = settings
	s.phaseDurations = durations
	s.clearReadiness()
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applySetGamePreset(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	if s.phase != PhaseWaiting && s.phase != PhaseFinished {
		return nil, ErrWrongPhase
	}

	settings, err := GameSettingsForPreset(event.GamePreset, s.baseDurations)
	if err != nil {
		return nil, err
	}
	durations, err := settings.ValidateForStartWithRuleSet(s.maxPlayers, s.baseDurations, s.gameRules())
	if err != nil {
		return nil, err
	}

	s.gameSettings = settings
	s.phaseDurations = durations
	s.clearReadiness()
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyReturnToWaiting(event Event) (*Envelope, error) {
	if event.PlayerID != s.ownerID {
		return nil, ErrOwnerOnly
	}
	if s.phase == PhaseWaiting {
		return nil, ErrAlreadyWaiting
	}

	for playerID, player := range s.players {
		if !player.Connected {
			delete(s.players, playerID)
			continue
		}
		player.Ready = false
		player.Role = ""
		player.Alive = true
		player.ShooterUsed = false
	}
	for spectatorID, spectator := range s.spectators {
		if !spectator.Connected {
			delete(s.spectators, spectatorID)
		}
	}

	s.ensureConnectedOwner()
	s.enterPhase(PhaseWaiting, event.At)
	s.round = 0
	s.nightActions = make(map[string]NightAction)
	s.votes = make(map[string]string)
	s.investigations = make(map[string][]InvestigationResult)
	s.lastWordsID = ""
	s.result = nil
	s.log = nil
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyPhaseTimeout(event Event) (*Envelope, error) {
	if s.phaseDeadline.IsZero() {
		return nil, ErrWrongPhase
	}
	if event.At.Before(s.phaseDeadline) {
		return nil, ErrPhaseDeadlineNotReached
	}

	expiredPhase := s.phase
	s.appendLog(LogPhaseTimedOut, event.At, LogEntry{Phase: expiredPhase})
	switch expiredPhase {
	case PhaseNight:
		for _, player := range s.players {
			if _, hasNightAction := s.gameRules().NightActionForRole(player.Role); player.Alive && hasNightAction && !s.nightActionSubmitted(player.ID) {
				s.nightActions[player.ID] = NightAction{
					PlayerID: player.ID,
					Type:     NightActionPass,
				}
			}
		}
		s.resolveNight(event.At)
	case PhaseDayDiscussion:
		s.startVoting(event.At, "")
	case PhaseDayVoting:
		for _, player := range s.players {
			if player.Alive {
				if _, voted := s.votes[player.ID]; !voted {
					s.votes[player.ID] = ""
				}
			}
		}
		s.resolveVote(event.At)
	case PhaseLastWords:
		s.completeLastWords(event.At)
	default:
		return nil, ErrWrongPhase
	}

	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) resolveNight(at time.Time) {
	blocked := make(map[string]bool)
	protected := make(map[string]bool)
	eliminated := make(map[string]bool)
	for _, action := range s.gameRules().OrderNightActions(s.nightActions) {
		if blocked[action.PlayerID] {
			continue
		}
		switch action.Type {
		case NightActionBlock:
			if action.TargetID != "" {
				blocked[action.TargetID] = true
			}
		case NightActionProtect:
			if action.TargetID != "" {
				protected[action.TargetID] = true
			}
		case NightActionKill:
			if action.TargetID != "" && !protected[action.TargetID] {
				if target, ok := s.players[action.TargetID]; ok && target.Alive {
					target.Alive = false
					eliminated[target.ID] = true
				}
			}
		case NightActionInvestigate:
			s.recordInvestigation(action)
		}
	}

	if len(eliminated) == 0 {
		s.appendLog(LogNightNoElimination, at, LogEntry{})
	} else {
		ids := sortedMapKeys(eliminated)
		for _, id := range ids {
			s.appendLog(LogNightEliminated, at, LogEntry{TargetID: id})
		}
	}

	s.nightActions = make(map[string]NightAction)
	s.pruneVotes()
	if s.checkWin(at) {
		return
	}

	s.enterPhase(PhaseDayDiscussion, at)
	s.votes = make(map[string]string)
	s.appendLog(LogDayStarted, at, LogEntry{})
}

func (s *State) resolveVote(at time.Time) {
	s.pruneVotes()

	tallies := make(map[string]int)
	for _, targetID := range s.votes {
		if targetID != "" {
			tallies[targetID]++
		}
	}

	targetID, tied := topVoteTarget(tallies)
	executedID := ""
	if targetID == "" || tied {
		s.appendLog(LogVoteNoExecution, at, LogEntry{})
	} else {
		if target, ok := s.players[targetID]; ok && target.Alive {
			target.Alive = false
			executedID = target.ID
			s.appendLog(LogPlayerExecuted, at, LogEntry{TargetID: target.ID})
		} else {
			s.appendLog(LogVoteNoExecution, at, LogEntry{})
		}
	}

	s.votes = make(map[string]string)
	if executedID != "" {
		s.lastWordsID = executedID
		s.enterPhase(PhaseLastWords, at)
		s.appendLog(LogLastWordsStarted, at, LogEntry{TargetID: executedID})
		return
	}
	s.advanceAfterDay(at)
}

func (s *State) completeLastWords(at time.Time) {
	s.lastWordsID = ""
	s.advanceAfterDay(at)
}

func (s *State) advanceAfterDay(at time.Time) {
	if s.checkWin(at) {
		return
	}

	s.round++
	s.enterPhase(PhaseNight, at)
	s.nightActions = make(map[string]NightAction)
	s.appendLog(LogNightStarted, at, LogEntry{})
}

func (s *State) recordInvestigation(action NightAction) {
	target, ok := s.players[action.TargetID]
	if !ok {
		return
	}

	s.investigations[action.PlayerID] = append(s.investigations[action.PlayerID], InvestigationResult{
		Round:      s.round,
		TargetID:   target.ID,
		TargetName: target.Name,
		Killer:     s.gameRules().IsKillerAligned(target.Role),
	})
}

func (s *State) checkWin(at time.Time) bool {
	if s.phase == PhaseWaiting || s.phase == PhaseFinished {
		return s.phase == PhaseFinished
	}

	killers := 0
	others := 0
	for _, player := range s.players {
		if !player.Alive {
			continue
		}
		if s.gameRules().IsKillerAligned(player.Role) {
			killers++
		} else {
			others++
		}
	}

	switch {
	case killers == 0:
		s.finish(WinnerVillagers, "all_killers_eliminated", at)
		return true
	case killers >= others:
		s.finish(WinnerKillers, "killers_control_the_room", at)
		return true
	default:
		return false
	}
}

func (s *State) finish(winner Winner, reason string, at time.Time) {
	s.enterPhase(PhaseFinished, at)
	s.result = &GameResult{
		Winner:     winner,
		Reason:     reason,
		FinishedAt: at.UTC(),
	}
	s.appendLog(LogGameFinished, at, LogEntry{
		Winner: winner,
		Reason: reason,
	})
}

func (s *State) nonOwnersReady() bool {
	for _, player := range s.players {
		if player.ID != s.ownerID && !player.Ready {
			return false
		}
	}
	return true
}

func (s *State) clearReadiness() {
	for _, player := range s.players {
		if player.ID != s.ownerID {
			player.Ready = false
		}
	}
}

func (s *State) nextOwnerID() string {
	ids := s.sortedPlayerIDs()
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func (s *State) ensureConnectedOwner() {
	if owner, ok := s.players[s.ownerID]; ok && owner.Connected {
		return
	}
	for _, playerID := range s.sortedPlayerIDs() {
		if s.players[playerID].Connected {
			s.ownerID = playerID
			return
		}
	}
}

func (s *State) livingPlayer(playerID string) (*Player, error) {
	player, ok := s.players[playerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}
	if !player.Alive {
		return nil, ErrPlayerDead
	}
	return player, nil
}

func (s *State) livingTarget(targetID string) (*Player, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil, ErrInvalidTarget
	}

	target, ok := s.players[targetID]
	if !ok || !target.Alive {
		return nil, ErrInvalidTarget
	}
	return target, nil
}

func (s *State) allNightActionsSubmitted() bool {
	for _, player := range s.players {
		if _, hasNightAction := s.gameRules().NightActionForRole(player.Role); player.Alive && hasNightAction && !s.nightActionSubmitted(player.ID) {
			return false
		}
	}
	return true
}

func (s *State) nightActionSubmitted(playerID string) bool {
	_, ok := s.nightActions[playerID]
	return ok
}

func (s *State) allLivingPlayersVoted() bool {
	living := 0
	for _, player := range s.players {
		if !player.Alive {
			continue
		}
		living++
		if _, ok := s.votes[player.ID]; !ok {
			return false
		}
	}
	return living > 0
}

func (s *State) pruneVotes() {
	for voterID, targetID := range s.votes {
		voter, voterOK := s.players[voterID]
		if !voterOK || !voter.Alive {
			delete(s.votes, voterID)
			continue
		}
		if targetID == "" {
			continue
		}
		target, targetOK := s.players[targetID]
		if !targetOK || !target.Alive {
			s.votes[voterID] = ""
		}
	}
}

func (s *State) sortedPlayerIDs() []string {
	ids := make([]string, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *State) appendLog(logType LogType, at time.Time, entry LogEntry) {
	entry.Type = logType
	entry.Round = s.round
	entry.At = at.UTC()
	s.log = append(s.log, entry)
	if len(s.log) > MaxEventLogEntries {
		s.log = s.log[len(s.log)-MaxEventLogEntries:]
	}
}

func (s *State) startVoting(at time.Time, playerID string) {
	s.enterPhase(PhaseDayVoting, at)
	s.votes = make(map[string]string)
	s.appendLog(LogVotingStarted, at, LogEntry{PlayerID: playerID})
}

func (s *State) enterPhase(phase Phase, at time.Time) {
	at = at.UTC()
	s.phase = phase
	s.phaseStartedAt = at
	duration := s.phaseDurations.durationFor(phase)
	if duration > 0 {
		s.phaseDeadline = at.Add(duration)
	} else {
		s.phaseDeadline = time.Time{}
	}
}

func (s *State) touch(at time.Time) {
	s.updatedAt = at.UTC()
}

func (s *State) lastClientSequence(playerID string) (uint64, bool) {
	if player, ok := s.players[playerID]; ok {
		return player.LastClientSequence, true
	}
	if spectator, ok := s.spectators[playerID]; ok {
		return spectator.LastClientSequence, true
	}
	return 0, false
}

func (s *State) recordClientSequence(playerID string, sequence uint64) {
	if player, ok := s.players[playerID]; ok {
		player.LastClientSequence = sequence
		return
	}
	if spectator, ok := s.spectators[playerID]; ok {
		spectator.LastClientSequence = sequence
	}
}

func stateEnvelope(snapshot Snapshot) *Envelope {
	return &Envelope{
		Type:  "state",
		State: &snapshot,
	}
}

func (s *State) gameRules() *RuleSet {
	if s.rules != nil {
		return s.rules
	}
	return DefaultRuleSet()
}

func isDayPhase(phase Phase) bool {
	return phase == PhaseDayDiscussion || phase == PhaseDayVoting
}

func shuffleRoles(roles []Role) error {
	for i := len(roles) - 1; i > 0; i-- {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(n.Int64())
		roles[i], roles[j] = roles[j], roles[i]
	}
	return nil
}

func newReconnectToken() (string, error) {
	var bytes [32]byte
	if _, err := cryptorand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func topVoteTarget(tallies map[string]int) (string, bool) {
	topTarget := ""
	topCount := 0
	tied := false

	ids := make([]string, 0, len(tallies))
	for id := range tallies {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		count := tallies[id]
		switch {
		case count > topCount:
			topTarget = id
			topCount = count
			tied = false
		case count == topCount:
			tied = true
		}
	}

	return topTarget, tied
}

func sortedMapKeys(values map[string]bool) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
