package room

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStateJoinReadyStartAssignsPrivateRoles(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "a", "b", "c", "d"})

	envelope, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"})
	if err != nil {
		t.Fatalf("start game: %v", err)
	}

	if envelope.State.Phase != PhaseNight {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseNight)
	}

	wantRoles := map[Role]bool{
		RoleVillager:  true,
		RoleKiller:    true,
		RoleDetective: true,
		RoleDoctor:    true,
		RoleShooter:   true,
	}
	for _, player := range state.players {
		delete(wantRoles, player.Role)
	}
	if len(wantRoles) != 0 {
		t.Fatalf("missing assigned roles: %v", wantRoles)
	}

	for _, player := range envelope.State.Players {
		if player.Role != "" {
			t.Fatalf("public snapshot leaked role for %s: %s", player.ID, player.Role)
		}
	}
	if private := state.PrivateView("owner"); private == nil || private.Role == "" {
		t.Fatalf("owner private view did not include assigned role: %#v", private)
	}
	if private := state.PrivateView("owner"); private == nil || private.ReconnectToken == "" {
		t.Fatalf("owner private view did not include reconnect token: %#v", private)
	}
}

func TestStateRejectsStartFromNonOwner(t *testing.T) {
	state := readyRoom(t)

	_, err := state.Apply(Event{Type: EventStartGame, PlayerID: "guest"})
	if !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("err = %v, want %v", err, ErrOwnerOnly)
	}
}

func TestStateRejectsStartUntilPlayersReady(t *testing.T) {
	state := NewState("room-1")
	_, _ = state.Apply(Event{Type: EventJoin, PlayerID: "owner", PlayerName: "Owner"})
	_, _ = state.Apply(Event{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest"})

	_, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner"})
	if !errors.Is(err, ErrPlayersNotReady) {
		t.Fatalf("err = %v, want %v", err, ErrPlayersNotReady)
	}
}

func TestStateRejectsNewJoinAfterStart(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})

	_, err := state.Apply(Event{Type: EventJoin, PlayerID: "late", PlayerName: "Late"})
	if !errors.Is(err, ErrRoomNotJoinable) {
		t.Fatalf("err = %v, want %v", err, ErrRoomNotJoinable)
	}
}

func TestStateTrimsChat(t *testing.T) {
	state := readyRoom(t)

	envelope, err := state.Apply(Event{
		Type:     EventChat,
		PlayerID: "guest",
		Message:  "  hello table  ",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	if envelope.Chat.Message != "hello table" {
		t.Fatalf("message = %q", envelope.Chat.Message)
	}
}

func TestStateRejectsLongChat(t *testing.T) {
	state := readyRoom(t)

	_, err := state.Apply(Event{
		Type:     EventChat,
		PlayerID: "guest",
		Message:  strings.Repeat("x", MaxChatBytes+1),
	})
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("err = %v, want %v", err, ErrMessageTooLong)
	}
}

func TestNightActionsResolveToDayAndKeepDetectiveResultPrivate(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner":     RoleKiller,
		"detective": RoleDetective,
		"doctor":    RoleDoctor,
		"villager":  RoleVillager,
	})

	if _, err := state.Apply(Event{Type: EventNightAction, PlayerID: "owner", TargetID: "villager"}); err != nil {
		t.Fatalf("killer action: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventNightAction, PlayerID: "detective", TargetID: "owner"}); err != nil {
		t.Fatalf("detective action: %v", err)
	}
	envelope, err := state.Apply(Event{Type: EventNightAction, PlayerID: "doctor", TargetID: "villager"})
	if err != nil {
		t.Fatalf("doctor action: %v", err)
	}

	if envelope.State.Phase != PhaseDayDiscussion {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseDayDiscussion)
	}
	if !state.players["villager"].Alive {
		t.Fatal("villager should be alive after doctor protection")
	}

	detectivePrivate := state.PrivateView("detective")
	if got := len(detectivePrivate.Investigations); got != 1 {
		t.Fatalf("investigations = %d, want 1", got)
	}
	if !detectivePrivate.Investigations[0].Killer {
		t.Fatal("detective result should identify killer target")
	}
	if ownerPrivate := state.PrivateView("owner"); len(ownerPrivate.Investigations) != 0 {
		t.Fatalf("owner should not see detective results: %#v", ownerPrivate.Investigations)
	}
	for _, player := range envelope.State.Players {
		if player.Role != "" {
			t.Fatalf("public snapshot leaked role for %s: %s", player.ID, player.Role)
		}
	}
}

func TestKillerWinsAfterNightKill(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})

	envelope, err := state.Apply(Event{Type: EventNightAction, PlayerID: "owner", TargetID: "guest"})
	if err != nil {
		t.Fatalf("killer action: %v", err)
	}

	if envelope.State.Phase != PhaseFinished {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseFinished)
	}
	if envelope.State.Result == nil || envelope.State.Result.Winner != WinnerKillers {
		t.Fatalf("result = %#v, want killer win", envelope.State.Result)
	}
	if role := playerView(t, envelope.State, "owner").Role; role != RoleKiller {
		t.Fatalf("finished snapshot should reveal killer role, got %s", role)
	}
}

func TestVoteExecutionCanFinishGame(t *testing.T) {
	state := gameStateWithRoles(PhaseDayVoting, map[string]Role{
		"owner":     RoleVillager,
		"killer":    RoleKiller,
		"detective": RoleDetective,
	})

	if _, err := state.Apply(Event{Type: EventVote, PlayerID: "owner", TargetID: "killer"}); err != nil {
		t.Fatalf("owner vote: %v", err)
	}
	if _, err := state.Apply(Event{Type: EventVote, PlayerID: "killer", TargetID: "owner"}); err != nil {
		t.Fatalf("killer vote: %v", err)
	}
	envelope, err := state.Apply(Event{Type: EventVote, PlayerID: "detective", TargetID: "killer"})
	if err != nil {
		t.Fatalf("detective vote: %v", err)
	}

	if envelope.State.Phase != PhaseLastWords || envelope.State.LastWordsID != "killer" {
		t.Fatalf("phase/last words = %s/%s, want %s/killer", envelope.State.Phase, envelope.State.LastWordsID, PhaseLastWords)
	}
	if envelope.State.Result != nil || envelope.State.PhaseDeadline == nil {
		t.Fatalf("result/deadline during last words = %#v/%v", envelope.State.Result, envelope.State.PhaseDeadline)
	}
	if got := envelope.State.PhaseDeadline.Sub(envelope.State.PhaseStartedAt); got != DefaultLastWordsDuration {
		t.Fatalf("last words duration = %s, want %s", got, DefaultLastWordsDuration)
	}
	assertLogContains(t, envelope.State.Log, LogLastWordsStarted, "")
	if _, err := state.Apply(Event{Type: EventChat, PlayerID: "owner", Message: "not my turn"}); !errors.Is(err, ErrLastWordsSpeakerOnly) {
		t.Fatalf("non-speaker chat err = %v, want %v", err, ErrLastWordsSpeakerOnly)
	}
	chat, err := state.Apply(Event{Type: EventChat, PlayerID: "killer", Message: "my last words"})
	if err != nil || chat.Chat == nil || chat.Chat.Message != "my last words" {
		t.Fatalf("speaker chat = %#v, err = %v", chat, err)
	}

	finished, err := state.Apply(Event{Type: EventPhaseTimeout, At: *envelope.State.PhaseDeadline})
	if err != nil {
		t.Fatalf("finish last words: %v", err)
	}
	if finished.State.Phase != PhaseFinished || finished.State.LastWordsID != "" {
		t.Fatalf("finished phase/last words = %s/%s", finished.State.Phase, finished.State.LastWordsID)
	}
	if finished.State.Result == nil || finished.State.Result.Winner != WinnerVillagers {
		t.Fatalf("result = %#v, want villagers win", finished.State.Result)
	}
}

func TestLastWordsTimeoutStartsNextNightWhenGameContinues(t *testing.T) {
	state := gameStateWithRoles(PhaseDayVoting, map[string]Role{
		"owner":     RoleVillager,
		"killer":    RoleKiller,
		"detective": RoleDetective,
		"doctor":    RoleDoctor,
	})
	state.phaseDurations = DefaultPhaseDurations()
	for playerID, targetID := range map[string]string{
		"owner":     "doctor",
		"killer":    "doctor",
		"detective": "doctor",
		"doctor":    "killer",
	} {
		envelope, err := state.Apply(Event{Type: EventVote, PlayerID: playerID, TargetID: targetID})
		if err != nil {
			t.Fatalf("vote by %s: %v", playerID, err)
		}
		if envelope.State.Phase == PhaseLastWords {
			deadline := *envelope.State.PhaseDeadline
			next, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline})
			if err != nil {
				t.Fatalf("finish last words: %v", err)
			}
			if next.State.Phase != PhaseNight || next.State.Round != 2 || next.State.LastWordsID != "" {
				t.Fatalf("phase/round/last words = %s/%d/%s", next.State.Phase, next.State.Round, next.State.LastWordsID)
			}
			return
		}
	}
	t.Fatal("votes did not enter last words")
}

func TestShooterCanEndGame(t *testing.T) {
	state := gameStateWithRoles(PhaseDayDiscussion, map[string]Role{
		"owner":    RoleShooter,
		"killer":   RoleKiller,
		"villager": RoleVillager,
	})

	envelope, err := state.Apply(Event{Type: EventShoot, PlayerID: "owner", TargetID: "killer"})
	if err != nil {
		t.Fatalf("shoot: %v", err)
	}

	if envelope.State.Phase != PhaseFinished {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseFinished)
	}
	if envelope.State.Result == nil || envelope.State.Result.Winner != WinnerVillagers {
		t.Fatalf("result = %#v, want villagers win", envelope.State.Result)
	}
	if !state.players["owner"].ShooterUsed {
		t.Fatal("shooter action should be marked used")
	}
}

func TestReconnectKeepsInGamePlayerAndRole(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})
	reconnectToken := state.players["guest"].ReconnectToken

	if _, err := state.Apply(Event{Type: EventLeave, PlayerID: "guest"}); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if state.players["guest"].Connected {
		t.Fatal("guest should be marked disconnected")
	}

	if _, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "guest",
		PlayerName:     "Guest Again",
		ReconnectToken: reconnectToken,
	}); err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	if !state.players["guest"].Connected {
		t.Fatal("guest should be reconnected")
	}
	if state.players["guest"].Role != RoleVillager {
		t.Fatalf("guest role = %s, want %s", state.players["guest"].Role, RoleVillager)
	}
}

func TestReconnectRejectsMissingToken(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})

	_, err := state.Apply(Event{Type: EventJoin, PlayerID: "guest", PlayerName: "Guest Again"})
	if !errors.Is(err, ErrReconnectTokenRequired) {
		t.Fatalf("err = %v, want %v", err, ErrReconnectTokenRequired)
	}
}

func TestReconnectRejectsInvalidToken(t *testing.T) {
	state := gameStateWithRoles(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})

	_, err := state.Apply(Event{
		Type:           EventJoin,
		PlayerID:       "guest",
		PlayerName:     "Guest Again",
		ReconnectToken: "wrong-token",
	})
	if !errors.Is(err, ErrInvalidReconnectToken) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidReconnectToken)
	}
}

func TestStartGamePublishesServerAuthoritativeNightDeadline(t *testing.T) {
	state := readyRoomWithPlayers(t, []string{"owner", "guest-1", "guest-2"})
	state.phaseDurations = PhaseDurations{
		Night:         45 * time.Second,
		DayDiscussion: time.Minute,
		DayVoting:     30 * time.Second,
		LastWords:     20 * time.Second,
	}
	state.gameSettings = StandardGameSettings(state.phaseDurations)
	startedAt := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)

	envelope, err := state.Apply(Event{Type: EventStartGame, PlayerID: "owner", At: startedAt})
	if err != nil {
		t.Fatalf("start game: %v", err)
	}

	if !envelope.State.PhaseStartedAt.Equal(startedAt) {
		t.Fatalf("phase started at = %s, want %s", envelope.State.PhaseStartedAt, startedAt)
	}
	if envelope.State.PhaseDeadline == nil {
		t.Fatal("night deadline is missing")
	}
	wantDeadline := startedAt.Add(45 * time.Second)
	if !envelope.State.PhaseDeadline.Equal(wantDeadline) {
		t.Fatalf("phase deadline = %s, want %s", envelope.State.PhaseDeadline, wantDeadline)
	}
	if envelope.State.ServerTime.IsZero() {
		t.Fatal("server time is missing")
	}
}

func TestStateRejectsPhaseTimeoutBeforeDeadline(t *testing.T) {
	state, deadline := timedGameState(PhaseNight, map[string]Role{
		"owner":     RoleKiller,
		"detective": RoleDetective,
		"villager":  RoleVillager,
	})

	_, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline.Add(-time.Nanosecond)})
	if !errors.Is(err, ErrPhaseDeadlineNotReached) {
		t.Fatalf("err = %v, want %v", err, ErrPhaseDeadlineNotReached)
	}
	if state.phase != PhaseNight {
		t.Fatalf("phase = %s, want unchanged %s", state.phase, PhaseNight)
	}
}

func TestNightTimeoutPassesMissingActionsAndStartsDiscussion(t *testing.T) {
	state, deadline := timedGameState(PhaseNight, map[string]Role{
		"owner":     RoleKiller,
		"detective": RoleDetective,
		"doctor":    RoleDoctor,
		"villager":  RoleVillager,
	})

	envelope, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline})
	if err != nil {
		t.Fatalf("expire night: %v", err)
	}
	if envelope.State.Phase != PhaseDayDiscussion {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseDayDiscussion)
	}
	for _, player := range state.players {
		if !player.Alive {
			t.Fatalf("player %s was eliminated even though every missing action should pass", player.ID)
		}
	}
	assertLogContains(t, envelope.State.Log, LogPhaseTimedOut, PhaseNight)
	assertLogContains(t, envelope.State.Log, LogNightNoElimination, "")
	if envelope.State.PhaseDeadline == nil || !envelope.State.PhaseDeadline.Equal(deadline.Add(time.Minute)) {
		t.Fatalf("discussion deadline = %v, want %s", envelope.State.PhaseDeadline, deadline.Add(time.Minute))
	}
}

func TestNightTimeoutPreservesSubmittedActionsAndPassesOnlyMissingPlayers(t *testing.T) {
	state, deadline := timedGameState(PhaseNight, map[string]Role{
		"owner":     RoleKiller,
		"detective": RoleDetective,
		"doctor":    RoleDoctor,
		"villager":  RoleVillager,
	})

	if _, err := state.Apply(Event{
		Type:     EventNightAction,
		PlayerID: "owner",
		TargetID: "villager",
		At:       deadline.Add(-time.Second),
	}); err != nil {
		t.Fatalf("submit killer action: %v", err)
	}

	envelope, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline})
	if err != nil {
		t.Fatalf("expire night: %v", err)
	}
	if state.players["villager"].Alive {
		t.Fatal("submitted killer action was replaced instead of being resolved")
	}
	if len(state.investigations["detective"]) != 0 {
		t.Fatal("missing detective action should have become a pass")
	}
	if envelope.State.Phase != PhaseDayDiscussion {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseDayDiscussion)
	}
	assertLogContains(t, envelope.State.Log, LogPhaseTimedOut, PhaseNight)
	assertLogContains(t, envelope.State.Log, LogNightEliminated, "")
}

func TestDiscussionTimeoutStartsVoting(t *testing.T) {
	state, deadline := timedGameState(PhaseDayDiscussion, map[string]Role{
		"owner":     RoleVillager,
		"killer":    RoleKiller,
		"detective": RoleDetective,
	})

	envelope, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline})
	if err != nil {
		t.Fatalf("expire discussion: %v", err)
	}
	if envelope.State.Phase != PhaseDayVoting {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseDayVoting)
	}
	assertLogContains(t, envelope.State.Log, LogPhaseTimedOut, PhaseDayDiscussion)
	assertLogContains(t, envelope.State.Log, LogVotingStarted, "")
}

func TestVotingTimeoutAbstainsMissingVotesAndStartsNextNight(t *testing.T) {
	state, deadline := timedGameState(PhaseDayVoting, map[string]Role{
		"owner":     RoleVillager,
		"killer":    RoleKiller,
		"detective": RoleDetective,
	})

	envelope, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline})
	if err != nil {
		t.Fatalf("expire voting: %v", err)
	}
	if envelope.State.Phase != PhaseNight || envelope.State.Round != 2 {
		t.Fatalf("phase/round = %s/%d, want %s/2", envelope.State.Phase, envelope.State.Round, PhaseNight)
	}
	assertLogContains(t, envelope.State.Log, LogPhaseTimedOut, PhaseDayVoting)
	assertLogContains(t, envelope.State.Log, LogVoteNoExecution, "")
	if envelope.State.PhaseDeadline == nil {
		t.Fatal("next night deadline is missing")
	}
}

func TestVotingTimeoutPreservesSubmittedVotesAndAbstainsOnlyMissingPlayers(t *testing.T) {
	state, deadline := timedGameState(PhaseDayVoting, map[string]Role{
		"owner":     RoleVillager,
		"killer":    RoleKiller,
		"detective": RoleDetective,
	})

	if _, err := state.Apply(Event{
		Type:     EventVote,
		PlayerID: "owner",
		TargetID: "detective",
		At:       deadline.Add(-time.Second),
	}); err != nil {
		t.Fatalf("submit vote: %v", err)
	}

	envelope, err := state.Apply(Event{Type: EventPhaseTimeout, At: deadline})
	if err != nil {
		t.Fatalf("expire voting: %v", err)
	}
	if state.players["detective"].Alive {
		t.Fatal("submitted vote was replaced instead of being counted")
	}
	if envelope.State.Phase != PhaseLastWords || envelope.State.LastWordsID != "detective" {
		t.Fatalf("phase/last words = %s/%s, want %s/detective", envelope.State.Phase, envelope.State.LastWordsID, PhaseLastWords)
	}
	assertLogContains(t, envelope.State.Log, LogPhaseTimedOut, PhaseDayVoting)
	assertLogContains(t, envelope.State.Log, LogPlayerExecuted, "")

	finished, err := state.Apply(Event{Type: EventPhaseTimeout, At: *envelope.State.PhaseDeadline})
	if err != nil {
		t.Fatalf("expire last words: %v", err)
	}
	if finished.State.Phase != PhaseFinished || finished.State.Result == nil || finished.State.Result.Winner != WinnerKillers {
		t.Fatalf("finished state = %#v", finished.State)
	}
	assertLogContains(t, finished.State.Log, LogPhaseTimedOut, PhaseLastWords)
}

func TestFinishedPhaseClearsDeadline(t *testing.T) {
	state, _ := timedGameState(PhaseNight, map[string]Role{
		"owner": RoleKiller,
		"guest": RoleVillager,
	})
	finishedAt := time.Now().UTC()

	envelope, err := state.Apply(Event{Type: EventNightAction, PlayerID: "owner", TargetID: "guest", At: finishedAt})
	if err != nil {
		t.Fatalf("killer action: %v", err)
	}
	if envelope.State.Phase != PhaseFinished {
		t.Fatalf("phase = %s, want %s", envelope.State.Phase, PhaseFinished)
	}
	if envelope.State.PhaseDeadline != nil {
		t.Fatalf("finished deadline = %s, want nil", envelope.State.PhaseDeadline)
	}
	if !envelope.State.PhaseStartedAt.Equal(finishedAt) {
		t.Fatalf("finished phase started at = %s, want %s", envelope.State.PhaseStartedAt, finishedAt)
	}
}

func readyRoom(t *testing.T) *State {
	t.Helper()
	return readyRoomWithPlayers(t, []string{"owner", "guest"})
}

func readyRoomWithPlayers(t *testing.T, ids []string) *State {
	t.Helper()

	state := NewState("room-1")
	for _, id := range ids {
		if _, err := state.Apply(Event{Type: EventJoin, PlayerID: id, PlayerName: id}); err != nil {
			t.Fatalf("join %s: %v", id, err)
		}
		if id != "owner" {
			if _, err := state.Apply(Event{Type: EventReady, PlayerID: id, Ready: true}); err != nil {
				t.Fatalf("ready %s: %v", id, err)
			}
		}
	}
	return state
}

func gameStateWithRoles(phase Phase, roles map[string]Role) *State {
	state := NewState("room-1")
	state.phase = phase
	state.round = 1
	state.ownerID = "owner"
	state.players = make(map[string]*Player)
	state.nightActions = make(map[string]NightAction)
	state.votes = make(map[string]string)
	state.investigations = make(map[string][]InvestigationResult)
	now := time.Now().UTC()

	for id, role := range roles {
		state.players[id] = &Player{
			ID:             id,
			Name:           id,
			ReconnectToken: "token-" + id,
			Ready:          true,
			Connected:      true,
			Role:           role,
			Alive:          true,
			JoinedAt:       now,
		}
	}
	return state
}

func timedGameState(phase Phase, roles map[string]Role) (*State, time.Time) {
	state := gameStateWithRoles(phase, roles)
	state.phaseDurations = PhaseDurations{
		Night:         30 * time.Second,
		DayDiscussion: time.Minute,
		DayVoting:     20 * time.Second,
		LastWords:     15 * time.Second,
	}
	startedAt := time.Date(2031, time.February, 3, 4, 5, 6, 0, time.UTC)
	state.enterPhase(phase, startedAt)
	return state, state.phaseDeadline
}

func assertLogContains(t *testing.T, entries []LogEntry, logType LogType, phase Phase) {
	t.Helper()
	for _, entry := range entries {
		if entry.Type == logType && (phase == "" || entry.Phase == phase) {
			return
		}
	}
	t.Fatalf("log does not contain type %s with phase %s: %#v", logType, phase, entries)
}

func playerView(t *testing.T, snapshot *Snapshot, playerID string) PlayerView {
	t.Helper()

	for _, player := range snapshot.Players {
		if player.ID == playerID {
			return player
		}
	}
	t.Fatalf("player %s not found in snapshot", playerID)
	return PlayerView{}
}
