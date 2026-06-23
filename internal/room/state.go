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
	MaxChatBytes       = 500
	MaxEventLogEntries = 100
)

var (
	ErrPlayerIDRequired       = errors.New("player id is required")
	ErrPlayerNameRequired     = errors.New("player name is required")
	ErrPlayerNotFound         = errors.New("player not found")
	ErrReconnectTokenRequired = errors.New("reconnect token is required")
	ErrInvalidReconnectToken  = errors.New("reconnect token is invalid")
	ErrOwnerOnly              = errors.New("only the room owner can perform this action")
	ErrRoomNotWaiting         = errors.New("room is not waiting")
	ErrRoomNotJoinable        = errors.New("room is not joinable")
	ErrWrongPhase             = errors.New("action is not allowed in the current phase")
	ErrGameFinished           = errors.New("game is already finished")
	ErrNotEnoughPlayers       = errors.New("not enough players to start")
	ErrPlayersNotReady        = errors.New("all non-owner players must be ready")
	ErrEmptyMessage           = errors.New("message is empty")
	ErrMessageTooLong         = errors.New("message is too long")
	ErrPlayerDead             = errors.New("player is dead")
	ErrInvalidTarget          = errors.New("target is invalid")
	ErrSelfTargetNotAllowed   = errors.New("self target is not allowed")
	ErrRoleHasNoNightAction   = errors.New("role has no night action")
	ErrShooterOnly            = errors.New("only the shooter can perform this action")
	ErrShooterAlreadyUsed     = errors.New("shooter action has already been used")
)

type State struct {
	roomID         string
	phase          Phase
	ownerID        string
	players        map[string]*Player
	round          int
	nightActions   map[string]NightAction
	votes          map[string]string
	investigations map[string][]InvestigationResult
	result         *GameResult
	log            []LogEntry
	updatedAt      time.Time
}

func NewState(roomID string) *State {
	now := time.Now().UTC()
	return &State{
		roomID:         roomID,
		phase:          PhaseWaiting,
		players:        make(map[string]*Player),
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
	case EventNightAction:
		return s.applyNightAction(event)
	case EventNightPass:
		return s.applyNightPass(event)
	case EventStartVote:
		return s.applyStartVote(event)
	case EventVote:
		return s.applyVote(event)
	case EventShoot:
		return s.applyShoot(event)
	default:
		return nil, fmt.Errorf("unknown room event: %s", event.Type)
	}
}

func (s *State) Snapshot() Snapshot {
	players := make([]PlayerView, 0, len(s.players))
	revealRoles := s.phase == PhaseFinished

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
		}
		if revealRoles {
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

	entries := make([]LogEntry, len(s.log))
	copy(entries, s.log)

	return Snapshot{
		RoomID:    s.roomID,
		OwnerID:   s.ownerID,
		Phase:     s.phase,
		Round:     s.round,
		Players:   players,
		Result:    s.result,
		Log:       entries,
		UpdatedAt: s.updatedAt,
	}
}

func (s *State) PrivateView(playerID string) *PrivatePlayerView {
	player, ok := s.players[playerID]
	if !ok {
		return nil
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

	if s.phase == PhaseNight && player.Alive && hasNightAction(player.Role) {
		view.ActionRequired = !s.nightActionSubmitted(player.ID)
		view.Available = append(view.Available, EventNightAction, EventNightPass)
	}

	if s.phase == PhaseDayVoting && player.Alive {
		view.CanVote = true
		view.VotedFor = s.votes[player.ID]
		view.Available = append(view.Available, EventVote)
	}

	if isDayPhase(s.phase) && player.Alive && player.Role == RoleShooter && !player.ShooterUsed {
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
		if strings.TrimSpace(event.ReconnectToken) == "" {
			return nil, ErrReconnectTokenRequired
		}
		if event.ReconnectToken != player.ReconnectToken {
			return nil, ErrInvalidReconnectToken
		}
		player.Name = strings.TrimSpace(event.PlayerName)
		player.Connected = true
		s.touch(event.At)
		return stateEnvelope(s.Snapshot()), nil
	}

	if s.phase != PhaseWaiting {
		return nil, ErrRoomNotJoinable
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
	if s.phase == PhaseFinished {
		return nil, ErrGameFinished
	}
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

	roles := roleDeck(len(s.players))
	if err := shuffleRoles(roles); err != nil {
		return nil, err
	}

	ids := s.sortedPlayerIDs()
	for i, id := range ids {
		player := s.players[id]
		player.Role = roles[i]
		player.Alive = true
		player.ShooterUsed = false
	}

	s.phase = PhaseNight
	s.round = 1
	s.nightActions = make(map[string]NightAction)
	s.votes = make(map[string]string)
	s.investigations = make(map[string][]InvestigationResult)
	s.result = nil
	s.log = nil
	s.appendLog(LogGameStarted, event.At, LogEntry{PlayerID: event.PlayerID})
	s.appendLog(LogNightStarted, event.At, LogEntry{})
	s.touch(event.At)
	return stateEnvelope(s.Snapshot()), nil
}

func (s *State) applyChat(event Event) (*Envelope, error) {
	player, ok := s.players[event.PlayerID]
	if !ok {
		return nil, ErrPlayerNotFound
	}
	if s.phase != PhaseWaiting && s.phase != PhaseFinished && !player.Alive {
		return nil, ErrPlayerDead
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

func (s *State) applyNightAction(event Event) (*Envelope, error) {
	if s.phase != PhaseNight {
		return nil, ErrWrongPhase
	}

	player, err := s.livingPlayer(event.PlayerID)
	if err != nil {
		return nil, err
	}

	actionType, err := nightActionForRole(player.Role)
	if err != nil {
		return nil, err
	}

	target, err := s.livingTarget(event.TargetID)
	if err != nil {
		return nil, err
	}

	switch actionType {
	case NightActionKill, NightActionInvestigate:
		if target.ID == player.ID {
			return nil, ErrSelfTargetNotAllowed
		}
	case NightActionProtect:
		// Doctors may protect themselves in this prototype.
	}

	s.nightActions[player.ID] = NightAction{
		PlayerID: player.ID,
		TargetID: target.ID,
		Type:     actionType,
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
	if !hasNightAction(player.Role) {
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

	s.phase = PhaseDayVoting
	s.votes = make(map[string]string)
	s.appendLog(LogVotingStarted, event.At, LogEntry{PlayerID: event.PlayerID})
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
	if shooter.Role != RoleShooter {
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

func (s *State) resolveNight(at time.Time) {
	protected := make(map[string]bool)
	for _, action := range s.sortedNightActions() {
		if action.Type == NightActionProtect {
			protected[action.TargetID] = true
		}
	}

	eliminated := make(map[string]bool)
	for _, action := range s.sortedNightActions() {
		switch action.Type {
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

	s.phase = PhaseDayDiscussion
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
	if targetID == "" || tied {
		s.appendLog(LogVoteNoExecution, at, LogEntry{})
	} else {
		if target, ok := s.players[targetID]; ok && target.Alive {
			target.Alive = false
			s.appendLog(LogPlayerExecuted, at, LogEntry{TargetID: target.ID})
		} else {
			s.appendLog(LogVoteNoExecution, at, LogEntry{})
		}
	}

	s.votes = make(map[string]string)
	if s.checkWin(at) {
		return
	}

	s.round++
	s.phase = PhaseNight
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
		Killer:     target.Role == RoleKiller,
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
		if player.Role == RoleKiller {
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
	s.phase = PhaseFinished
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

func (s *State) nextOwnerID() string {
	ids := s.sortedPlayerIDs()
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
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
		if player.Alive && hasNightAction(player.Role) && !s.nightActionSubmitted(player.ID) {
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

func (s *State) sortedNightActions() []NightAction {
	actions := make([]NightAction, 0, len(s.nightActions))
	for _, action := range s.nightActions {
		actions = append(actions, action)
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].PlayerID < actions[j].PlayerID
	})
	return actions
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

func (s *State) touch(at time.Time) {
	s.updatedAt = at.UTC()
}

func stateEnvelope(snapshot Snapshot) *Envelope {
	return &Envelope{
		Type:  "state",
		State: &snapshot,
	}
}

func hasNightAction(role Role) bool {
	switch role {
	case RoleKiller, RoleDetective, RoleDoctor:
		return true
	default:
		return false
	}
}

func nightActionForRole(role Role) (NightActionType, error) {
	switch role {
	case RoleKiller:
		return NightActionKill, nil
	case RoleDetective:
		return NightActionInvestigate, nil
	case RoleDoctor:
		return NightActionProtect, nil
	default:
		return "", ErrRoleHasNoNightAction
	}
}

func isDayPhase(phase Phase) bool {
	return phase == PhaseDayDiscussion || phase == PhaseDayVoting
}

func roleDeck(playerCount int) []Role {
	roles := []Role{RoleKiller, RoleVillager}
	if playerCount >= 3 {
		roles = append(roles, RoleDetective)
	}
	if playerCount >= 4 {
		roles = append(roles, RoleDoctor)
	}
	if playerCount >= 5 {
		roles = append(roles, RoleShooter)
	}
	for len(roles) < playerCount {
		roles = append(roles, RoleVillager)
	}
	return roles
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
