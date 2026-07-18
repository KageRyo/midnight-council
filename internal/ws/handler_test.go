package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"midnight-council/internal/moderation"
	"midnight-council/internal/room"
)

type testClient struct {
	id   string
	conn *websocket.Conn
}

type wireEnvelope struct {
	Type    string                  `json:"type"`
	State   *room.Snapshot          `json:"state,omitempty"`
	Chat    *room.ChatMessage       `json:"chat,omitempty"`
	Ack     *room.EventAck          `json:"ack,omitempty"`
	Error   string                  `json:"error,omitempty"`
	Private *room.PrivatePlayerView `json:"private,omitempty"`
}

func TestHandlerPlaysFullGameOverWebSocket(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	clients := []*testClient{
		connectClient(t, server.URL, "p1", "Player 1"),
		connectClient(t, server.URL, "p2", "Player 2"),
		connectClient(t, server.URL, "p3", "Player 3"),
		connectClient(t, server.URL, "p4", "Player 4"),
		connectClient(t, server.URL, "p5", "Player 5"),
	}
	defer closeClients(clients)

	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == len(clients)
	})

	for _, client := range clients[1:] {
		writeClientEvent(t, client, map[string]any{"type": "ready", "ready": true})
	}

	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return nonOwnerPlayersReady(state)
	})

	writeClientEvent(t, clients[0], map[string]any{"type": "start_game"})

	byRole := make(map[room.Role]*testClient)
	for _, client := range clients {
		envelope := readStateUntil(t, client, func(state *room.Snapshot, private *room.PrivatePlayerView) bool {
			return state.Phase == room.PhaseNight && private != nil && private.Role != ""
		})

		for _, player := range envelope.State.Players {
			if player.Role != "" {
				t.Fatalf("public state leaked role for %s: %s", player.ID, player.Role)
			}
		}
		byRole[envelope.Private.Role] = client
	}

	killer := requireRoleClient(t, byRole, room.RoleKiller)
	detective := requireRoleClient(t, byRole, room.RoleDetective)
	doctor := requireRoleClient(t, byRole, room.RoleDoctor)
	villager := requireRoleClient(t, byRole, room.RoleVillager)

	writeClientEvent(t, killer, map[string]any{"type": "night_action", "target_id": villager.id})
	writeClientEvent(t, detective, map[string]any{"type": "night_action", "target_id": killer.id})
	writeClientEvent(t, doctor, map[string]any{"type": "night_action", "target_id": villager.id})

	detectiveDay := readStateUntil(t, detective, func(state *room.Snapshot, private *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseDayDiscussion && private != nil && len(private.Investigations) == 1
	})
	if !detectiveDay.Private.Investigations[0].Killer {
		t.Fatal("detective private result should identify the killer")
	}
	if !playerAlive(t, detectiveDay.State, villager.id) {
		t.Fatal("doctor-protected villager should still be alive")
	}

	writeClientEvent(t, clients[0], map[string]any{"type": "start_vote"})
	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseDayVoting
	})

	nonKillerTarget := firstClientIDExcept(clients, killer.id)
	for _, client := range clients {
		targetID := killer.id
		if client.id == killer.id {
			targetID = nonKillerTarget
		}
		writeClientEvent(t, client, map[string]any{"type": "vote", "target_id": targetID})
	}

	finished := readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseFinished
	})
	if finished.State.Result == nil || finished.State.Result.Winner != room.WinnerVillagers {
		t.Fatalf("result = %#v, want villagers win", finished.State.Result)
	}
	if role := playerRole(t, finished.State, killer.id); role != room.RoleKiller {
		t.Fatalf("finished public state should reveal killer role, got %s", role)
	}
}

func TestHandlerRejectsInvalidClientEventSchema(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	client := connectClient(t, server.URL, "p1", "Player 1")
	defer client.conn.Close()

	if err := client.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"start_game","target_id":"p2"}`)); err != nil {
		t.Fatalf("write invalid event: %v", err)
	}

	envelope := readEnvelopeUntil(t, client, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && strings.Contains(envelope.Error, "does not accept target_id")
	})
	if envelope.Error == "" {
		t.Fatal("expected schema error envelope")
	}
}

func TestHandlerRequiresReconnectTokenForExistingPlayer(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	original := connectClient(t, server.URL, "p1", "Player 1")
	defer original.conn.Close()

	joined := readStateUntil(t, original, func(_ *room.Snapshot, private *room.PrivatePlayerView) bool {
		return private != nil && private.ReconnectToken != ""
	})
	reconnectToken := joined.Private.ReconnectToken

	rejected := connectClient(t, server.URL, "p1", "Impostor")
	defer rejected.conn.Close()
	readEnvelopeUntil(t, rejected, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && strings.Contains(envelope.Error, "reconnect token is required")
	})

	reconnected := connectClientWithToken(t, server.URL, "p1", "Player 1 Again", reconnectToken)
	defer reconnected.conn.Close()
	readStateUntil(t, reconnected, func(_ *room.Snapshot, private *room.PrivatePlayerView) bool {
		return private != nil && private.ReconnectToken == reconnectToken
	})
}

func TestHandlerReplacesExistingConnectionForSameSeat(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	original := connectClient(t, server.URL, "player", "Player")
	defer original.conn.Close()
	joined := readStateUntil(t, original, func(_ *room.Snapshot, private *room.PrivatePlayerView) bool {
		return private != nil && private.ReconnectToken != ""
	})

	replacement := connectClientWithToken(t, server.URL, "player", "Player New", joined.Private.ReconnectToken)
	defer replacement.conn.Close()
	readStateUntil(t, replacement, func(state *room.Snapshot, private *room.PrivatePlayerView) bool {
		return len(state.Players) == 1 && state.Players[0].Connected && private != nil
	})

	readEnvelopeUntil(t, original, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == room.ErrConnectionReplaced.Error()
	})
	assertPolicyViolationClose(t, original, room.ErrConnectionReplaced.Error())
}

func TestHandlerPreservesNetworkDisconnectButRemovesExplicitWaitingLeave(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	owner := connectClient(t, server.URL, "owner", "Owner")
	guest := connectClient(t, server.URL, "guest", "Guest")
	defer owner.conn.Close()
	defer guest.conn.Close()

	joined := readStateUntil(t, guest, func(state *room.Snapshot, private *room.PrivatePlayerView) bool {
		return len(state.Players) == 2 && private != nil && private.ReconnectToken != ""
	})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 2
	})

	if err := guest.conn.Close(); err != nil {
		t.Fatalf("drop guest connection: %v", err)
	}
	disconnected := readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 2 && !playerConnected(t, state, "guest")
	})
	if playerConnected(t, disconnected.State, "guest") {
		t.Fatal("network disconnect removed offline indication")
	}

	reconnected := connectClientWithToken(t, server.URL, "guest", "Guest Again", joined.Private.ReconnectToken)
	defer reconnected.conn.Close()
	readStateUntil(t, reconnected, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 2 && playerConnected(t, state, "guest")
	})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 2 && playerConnected(t, state, "guest")
	})

	if err := reconnected.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "player left"),
		time.Now().Add(time.Second),
	); err != nil {
		t.Fatalf("write explicit leave close: %v", err)
	}
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 1
	})
}

func TestHandlerAcknowledgesAndDeduplicatesSequencedEvents(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	owner := connectClient(t, server.URL, "owner", "Owner")
	guest := connectClient(t, server.URL, "guest", "Guest")
	defer closeClients([]*testClient{owner, guest})
	for _, client := range []*testClient{owner, guest} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}

	writeClientEvent(t, guest, map[string]any{"type": "ready", "sequence": 1, "ready": true})
	readAckUntil(t, guest, 1, "applied")
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return playerReady(t, state, "guest")
	})

	writeClientEvent(t, guest, map[string]any{"type": "ready", "sequence": 1, "ready": false})
	readAckUntil(t, guest, 1, "duplicate")
	writeClientEvent(t, guest, map[string]any{"type": "presence", "sequence": 2, "afk": true})
	readAckUntil(t, guest, 2, "applied")
	presence := readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return playerAFK(t, state, "guest")
	})
	if !playerReady(t, presence.State, "guest") {
		t.Fatal("duplicate sequence changed guest readiness")
	}

	writeClientEvent(t, guest, map[string]any{"type": "start_game", "sequence": 3})
	readAckUntil(t, guest, 3, "rejected")
}

func TestHandlerBroadcastsAutomaticPhaseProgression(t *testing.T) {
	durations := room.PhaseDurations{
		Night:         60 * time.Millisecond,
		DayDiscussion: 80 * time.Millisecond,
		DayVoting:     60 * time.Millisecond,
	}
	server := httptest.NewServer(NewHandler(room.NewHub(room.WithPhaseDurations(durations))))
	defer server.Close()

	clients := []*testClient{
		connectClient(t, server.URL, "p1", "Player 1"),
		connectClient(t, server.URL, "p2", "Player 2"),
		connectClient(t, server.URL, "p3", "Player 3"),
	}
	defer closeClients(clients)

	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == len(clients)
	})
	for _, client := range clients[1:] {
		writeClientEvent(t, client, map[string]any{"type": "ready", "ready": true})
	}
	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return nonOwnerPlayersReady(state)
	})
	writeClientEvent(t, clients[0], map[string]any{"type": "start_game"})

	night := readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseNight && state.Round == 1
	})
	assertSnapshotDeadline(t, night.State, durations.Night)

	discussion := readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseDayDiscussion
	})
	assertSnapshotDeadline(t, discussion.State, durations.DayDiscussion)

	voting := readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseDayVoting
	})
	assertSnapshotDeadline(t, voting.State, durations.DayVoting)

	nextNight := readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseNight && state.Round == 2
	})
	assertSnapshotDeadline(t, nextNight.State, durations.Night)
}

func TestHandlerRateLimitsChatPerConnectionAndRecovers(t *testing.T) {
	limits := EventRateLimits{
		Chat: RateLimit{EventsPerSecond: 20, Burst: 1},
		Game: RateLimit{EventsPerSecond: 100, Burst: 10},
	}
	var reviewCount atomic.Int32
	policy := moderation.ChatPolicyFunc(func(_ context.Context, _ moderation.ChatRequest) (moderation.ChatDecision, error) {
		reviewCount.Add(1)
		return moderation.ChatDecision{Action: moderation.ChatAllow}, nil
	})
	server := httptest.NewServer(NewHandler(
		room.NewHub(),
		WithEventRateLimits(limits),
		WithChatPolicy(policy),
	))
	defer server.Close()

	sender := connectClient(t, server.URL, "sender", "Sender")
	observer := connectClient(t, server.URL, "observer", "Observer")
	defer closeClients([]*testClient{sender, observer})

	for _, client := range []*testClient{sender, observer} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}

	writeClientEvent(t, sender, map[string]any{"type": "chat", "message": "first"})
	assertNextChat(t, sender, "sender", "first")
	assertNextChat(t, observer, "sender", "first")

	writeClientEvent(t, sender, map[string]any{"type": "chat", "message": "limited"})
	readEnvelopeUntil(t, sender, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == chatRateLimitError
	})
	if got := reviewCount.Load(); got != 1 {
		t.Fatalf("moderation review count = %d, want 1; rate limit should run first", got)
	}

	writeClientEvent(t, observer, map[string]any{"type": "chat", "message": "independent"})
	assertNextChat(t, observer, "observer", "independent")
	assertNextChat(t, sender, "observer", "independent")

	time.Sleep(75 * time.Millisecond)
	writeClientEvent(t, sender, map[string]any{"type": "chat", "message": "recovered"})
	assertNextChat(t, observer, "sender", "recovered")
}

func TestHandlerAppliesChatModerationBeforeRoomDispatch(t *testing.T) {
	requests := make(chan moderation.ChatRequest, 4)
	policy := moderation.ChatPolicyFunc(func(_ context.Context, request moderation.ChatRequest) (moderation.ChatDecision, error) {
		requests <- request
		switch request.Message {
		case "replace":
			return moderation.ChatDecision{
				Action:      moderation.ChatReplace,
				Replacement: "filtered",
			}, nil
		case "reject":
			return moderation.ChatDecision{
				Action: moderation.ChatReject,
				Reason: "blocked by test policy",
			}, nil
		default:
			return moderation.ChatDecision{Action: moderation.ChatAllow}, nil
		}
	})
	limits := EventRateLimits{
		Chat: RateLimit{EventsPerSecond: 100, Burst: 10},
		Game: RateLimit{EventsPerSecond: 100, Burst: 10},
	}
	server := httptest.NewServer(NewHandler(
		room.NewHub(),
		WithEventRateLimits(limits),
		WithChatPolicy(policy),
	))
	defer server.Close()

	sender := connectClient(t, server.URL, "sender", "Sender")
	observer := connectClient(t, server.URL, "observer", "Observer")
	defer closeClients([]*testClient{sender, observer})

	for _, client := range []*testClient{sender, observer} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}

	writeClientEvent(t, sender, map[string]any{"type": "chat", "message": "replace"})
	assertNextChat(t, sender, "sender", "filtered")
	assertNextChat(t, observer, "sender", "filtered")
	request := <-requests
	if request.RoomID != "integration" || request.PlayerID != "sender" || request.PlayerName != "Sender" || request.Message != "replace" {
		t.Fatalf("moderation request = %#v, want sender metadata and original message", request)
	}

	writeClientEvent(t, sender, map[string]any{"type": "chat", "message": "reject"})
	readEnvelopeUntil(t, sender, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == "blocked by test policy"
	})

	writeClientEvent(t, sender, map[string]any{"type": "chat", "message": "after rejection"})
	assertNextChat(t, observer, "sender", "after rejection")
	assertNextChat(t, sender, "sender", "after rejection")
}

func TestHandlerRateLimitsGameEventsBeforeRoomDispatch(t *testing.T) {
	limits := EventRateLimits{
		Chat: RateLimit{EventsPerSecond: 100, Burst: 1},
		Game: RateLimit{EventsPerSecond: 0.01, Burst: 1},
	}
	server := httptest.NewServer(NewHandler(room.NewHub(), WithEventRateLimits(limits)))
	defer server.Close()

	owner := connectClient(t, server.URL, "owner", "Owner")
	guest := connectClient(t, server.URL, "guest", "Guest")
	defer closeClients([]*testClient{owner, guest})

	for _, client := range []*testClient{owner, guest} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}

	writeClientEvent(t, guest, map[string]any{"type": "ready", "ready": true})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return playerReady(t, state, "guest")
	})

	writeClientEvent(t, guest, map[string]any{"type": "ready", "ready": false})
	readEnvelopeUntil(t, guest, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == gameRateLimitError
	})

	probe := connectClient(t, server.URL, "probe", "Probe")
	defer probe.conn.Close()
	probeState := readStateUntil(t, probe, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 3
	})
	if !playerReady(t, probeState.State, "guest") {
		t.Fatal("rate-limited ready event reached the room actor")
	}

	writeClientEvent(t, probe, map[string]any{"type": "ready", "ready": true})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return playerReady(t, state, "probe")
	})

	writeClientEvent(t, guest, map[string]any{"type": "chat", "message": "separate bucket"})
	assertNextChat(t, owner, "guest", "separate bucket")
}

func TestHandlerSupportsRoomAdministration(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	owner := connectClient(t, server.URL, "owner", "Owner")
	guest := connectClient(t, server.URL, "guest", "Guest")
	defer closeClients([]*testClient{owner, guest})

	for _, client := range []*testClient{owner, guest} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}

	writeClientEvent(t, owner, map[string]any{"type": "set_player_limit", "max_players": 2})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.MaxPlayers == 2
	})
	writeClientEvent(t, owner, map[string]any{"type": "set_room_locked", "locked": true})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Locked
	})

	lockedOut := connectClient(t, server.URL, "locked-out", "Locked Out")
	defer lockedOut.conn.Close()
	readEnvelopeUntil(t, lockedOut, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == room.ErrRoomLocked.Error()
	})
	assertPolicyViolationClose(t, lockedOut, room.ErrRoomLocked.Error())

	writeClientEvent(t, owner, map[string]any{"type": "set_room_locked", "locked": false})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return !state.Locked
	})
	fullRoom := connectClient(t, server.URL, "full-room", "Full Room")
	defer fullRoom.conn.Close()
	readEnvelopeUntil(t, fullRoom, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == room.ErrRoomFull.Error()
	})
	assertPolicyViolationClose(t, fullRoom, room.ErrRoomFull.Error())

	writeClientEvent(t, owner, map[string]any{"type": "transfer_owner", "target_id": "guest"})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.OwnerID == "guest"
	})
}

func TestHandlerSupportsActiveSpectatorsAndReturnToWaiting(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	owner := connectClient(t, server.URL, "owner", "Owner")
	guest := connectClient(t, server.URL, "guest", "Guest")
	defer closeClients([]*testClient{owner, guest})

	for _, client := range []*testClient{owner, guest} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}
	writeClientEvent(t, guest, map[string]any{"type": "ready", "ready": true})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return playerReady(t, state, "guest")
	})
	writeClientEvent(t, owner, map[string]any{"type": "start_game"})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseNight
	})

	spectator := connectSpectator(t, server.URL, "spectator", "Spectator")
	defer spectator.conn.Close()
	observing := readStateUntil(t, spectator, func(state *room.Snapshot, private *room.PrivatePlayerView) bool {
		return state.Phase == room.PhaseNight && private != nil && private.Spectator
	})
	if len(observing.State.Players) != 2 || len(observing.State.Spectators) != 1 {
		t.Fatalf("players/spectators = %d/%d, want 2/1", len(observing.State.Players), len(observing.State.Spectators))
	}

	writeClientEvent(t, spectator, map[string]any{"type": "chat", "message": "active spoiler"})
	readEnvelopeUntil(t, spectator, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == room.ErrSpectatorCannotChat.Error()
	})

	writeClientEvent(t, owner, map[string]any{"type": "return_to_waiting"})
	for _, client := range []*testClient{owner, spectator} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return state.Phase == room.PhaseWaiting
		})
	}
	writeClientEvent(t, spectator, map[string]any{"type": "chat", "message": "hello after reset"})
	assertNextChat(t, owner, "spectator", "hello after reset")
}

func TestHandlerClosesKickedParticipantConnection(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	owner := connectClient(t, server.URL, "owner", "Owner")
	guest := connectClient(t, server.URL, "guest", "Guest")
	defer closeClients([]*testClient{owner, guest})

	for _, client := range []*testClient{owner, guest} {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 2
		})
	}

	writeClientEvent(t, owner, map[string]any{"type": "kick_participant", "target_id": "guest"})
	readStateUntil(t, owner, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return len(state.Players) == 1
	})
	readEnvelopeUntil(t, guest, func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == room.ErrKicked.Error()
	})

	if err := guest.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set kicked connection deadline: %v", err)
	}
	if _, _, err := guest.conn.ReadMessage(); err == nil {
		t.Fatal("kicked participant connection remained open")
	}
}

func TestHandlerSupportsServerValidatedGameSettings(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub()))
	defer server.Close()

	clients := []*testClient{
		connectClient(t, server.URL, "owner", "Owner"),
		connectClient(t, server.URL, "guest-1", "Guest 1"),
		connectClient(t, server.URL, "guest-2", "Guest 2"),
		connectClient(t, server.URL, "guest-3", "Guest 3"),
	}
	defer closeClients(clients)

	for _, client := range clients {
		readStateUntil(t, client, func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
			return len(state.Players) == 4
		})
	}
	for _, client := range clients[1:] {
		writeClientEvent(t, client, map[string]any{"type": "ready", "ready": true})
	}
	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return nonOwnerPlayersReady(state)
	})

	writeClientEvent(t, clients[0], map[string]any{
		"type":                    "set_game_settings",
		"night_duration":          "30s",
		"day_discussion_duration": "40s",
		"day_voting_duration":     "30s",
		"minimum_players":         4,
		"reveal_roles_on_death":   true,
		"killers":                 1,
		"detectives":              0,
		"doctors":                 0,
		"shooters":                0,
	})
	settingsState := readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return state.GameSettings.Preset == room.GamePresetCustom &&
			state.GameSettings.NightDuration == "30s"
	})
	if nonOwnerPlayersReady(settingsState.State) {
		t.Fatal("settings change did not reset readiness")
	}

	writeClientEvent(t, clients[0], map[string]any{
		"type":                    "set_game_settings",
		"night_duration":          "30s",
		"day_discussion_duration": "40s",
		"day_voting_duration":     "30s",
		"minimum_players":         3,
		"reveal_roles_on_death":   false,
		"killers":                 2,
		"detectives":              0,
		"doctors":                 0,
		"shooters":                0,
	})
	readEnvelopeUntil(t, clients[0], func(envelope wireEnvelope) bool {
		return envelope.Type == "error" && envelope.Error == room.ErrInvalidRoleConfiguration.Error()
	})

	for _, client := range clients[1:] {
		writeClientEvent(t, client, map[string]any{"type": "ready", "ready": true})
	}
	readStateUntil(t, clients[0], func(state *room.Snapshot, _ *room.PrivatePlayerView) bool {
		return nonOwnerPlayersReady(state)
	})
	writeClientEvent(t, clients[0], map[string]any{"type": "start_game"})

	roleCounts := make(map[room.Role]int)
	for _, client := range clients {
		envelope := readStateUntil(t, client, func(state *room.Snapshot, private *room.PrivatePlayerView) bool {
			return state.Phase == room.PhaseNight && private != nil && private.Role != ""
		})
		roleCounts[envelope.Private.Role]++
		assertSnapshotDeadline(t, envelope.State, 30*time.Second)
	}
	if roleCounts[room.RoleKiller] != 1 || roleCounts[room.RoleVillager] != 3 || len(roleCounts) != 2 {
		t.Fatalf("role counts = %#v", roleCounts)
	}
}

func assertSnapshotDeadline(t *testing.T, state *room.Snapshot, duration time.Duration) {
	t.Helper()
	if state.PhaseStartedAt.IsZero() {
		t.Fatal("phase_started_at is missing")
	}
	if state.PhaseDeadline == nil {
		t.Fatal("phase_deadline is missing")
	}
	if got := state.PhaseDeadline.Sub(state.PhaseStartedAt); got != duration {
		t.Fatalf("phase duration = %s, want %s", got, duration)
	}
	if state.ServerTime.IsZero() {
		t.Fatal("server_time is missing")
	}
}

func connectClient(t *testing.T, serverURL, playerID, playerName string) *testClient {
	t.Helper()
	return connectParticipant(t, serverURL, playerID, playerName, "", false)
}

func connectClientWithToken(t *testing.T, serverURL, playerID, playerName, reconnectToken string) *testClient {
	t.Helper()
	return connectParticipant(t, serverURL, playerID, playerName, reconnectToken, false)
}

func connectSpectator(t *testing.T, serverURL, playerID, playerName string) *testClient {
	t.Helper()
	return connectParticipant(t, serverURL, playerID, playerName, "", true)
}

func connectParticipant(t *testing.T, serverURL, playerID, playerName, reconnectToken string, spectator bool) *testClient {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	parsed.Scheme = "ws"
	parsed.Path = "/ws/rooms/integration"
	query := parsed.Query()
	query.Set("player_id", playerID)
	query.Set("name", playerName)
	if reconnectToken != "" {
		query.Set("reconnect_token", reconnectToken)
	}
	if spectator {
		query.Set("spectator", "true")
	}
	parsed.RawQuery = query.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(parsed.String(), nil)
	if err != nil {
		t.Fatalf("dial %s: %v", playerID, err)
	}

	return &testClient{id: playerID, conn: conn}
}

func writeClientEvent(t *testing.T, client *testClient, event map[string]any) {
	t.Helper()

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event for %s: %v", client.id, err)
	}
	if err := client.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write event for %s: %v", client.id, err)
	}
}

func readStateUntil(t *testing.T, client *testClient, accept func(*room.Snapshot, *room.PrivatePlayerView) bool) wireEnvelope {
	t.Helper()

	return readEnvelopeUntil(t, client, func(envelope wireEnvelope) bool {
		return envelope.Type == "state" && envelope.State != nil && accept(envelope.State, envelope.Private)
	})
}

func readEnvelopeUntil(t *testing.T, client *testClient, accept func(wireEnvelope) bool) wireEnvelope {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		envelope := readEnvelope(t, client, deadline)
		if envelope.Type == "error" && !accept(envelope) {
			t.Fatalf("unexpected error envelope for %s: %s", client.id, envelope.Error)
		}
		if accept(envelope) {
			return envelope
		}
	}

	t.Fatalf("timed out waiting for envelope for %s", client.id)
	return wireEnvelope{}
}

func readEnvelope(t *testing.T, client *testClient, deadline time.Time) wireEnvelope {
	t.Helper()

	if err := client.conn.SetReadDeadline(deadline); err != nil {
		t.Fatalf("set read deadline for %s: %v", client.id, err)
	}

	var envelope wireEnvelope
	if err := client.conn.ReadJSON(&envelope); err != nil {
		t.Fatalf("read envelope for %s: %v", client.id, err)
	}
	return envelope
}

func readAckUntil(t *testing.T, client *testClient, sequence uint64, status string) wireEnvelope {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		envelope := readEnvelope(t, client, deadline)
		if envelope.Type == "ack" && envelope.Ack != nil &&
			envelope.Ack.Sequence == sequence && envelope.Ack.Status == status {
			return envelope
		}
		if envelope.Type == "error" && status != "rejected" {
			t.Fatalf("unexpected error envelope for %s: %s", client.id, envelope.Error)
		}
	}
}

func assertNextChat(t *testing.T, client *testClient, playerID, message string) {
	t.Helper()
	envelope := readEnvelope(t, client, time.Now().Add(5*time.Second))
	if envelope.Type != "chat" || envelope.Chat == nil {
		t.Fatalf("envelope = %#v, want chat", envelope)
	}
	if envelope.Chat.PlayerID != playerID || envelope.Chat.Message != message {
		t.Fatalf("chat = %#v, want player %s message %q", envelope.Chat, playerID, message)
	}
}

func assertPolicyViolationClose(t *testing.T, client *testClient, reason string) {
	t.Helper()

	if err := client.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set close deadline for %s: %v", client.id, err)
	}
	_, _, err := client.conn.ReadMessage()
	if err == nil {
		t.Fatalf("connection for %s remained open", client.id)
	}
	var closeError *websocket.CloseError
	if !errors.As(err, &closeError) {
		t.Fatalf("close error for %s = %v, want WebSocket close frame", client.id, err)
	}
	if closeError.Code != websocket.ClosePolicyViolation || closeError.Text != reason {
		t.Fatalf(
			"close frame for %s = code %d reason %q, want code %d reason %q",
			client.id,
			closeError.Code,
			closeError.Text,
			websocket.ClosePolicyViolation,
			reason,
		)
	}
}

func requireRoleClient(t *testing.T, byRole map[room.Role]*testClient, role room.Role) *testClient {
	t.Helper()

	client, ok := byRole[role]
	if !ok {
		t.Fatalf("missing client with role %s", role)
	}
	return client
}

func nonOwnerPlayersReady(state *room.Snapshot) bool {
	if len(state.Players) == 0 {
		return false
	}
	for _, player := range state.Players {
		if !player.Owner && !player.Ready {
			return false
		}
	}
	return true
}

func playerAlive(t *testing.T, state *room.Snapshot, playerID string) bool {
	t.Helper()

	for _, player := range state.Players {
		if player.ID == playerID {
			return player.Alive
		}
	}
	t.Fatalf("player %s missing from state", playerID)
	return false
}

func playerReady(t *testing.T, state *room.Snapshot, playerID string) bool {
	t.Helper()

	for _, player := range state.Players {
		if player.ID == playerID {
			return player.Ready
		}
	}
	t.Fatalf("player %s missing from state", playerID)
	return false
}

func playerConnected(t *testing.T, state *room.Snapshot, playerID string) bool {
	t.Helper()

	for _, player := range state.Players {
		if player.ID == playerID {
			return player.Connected
		}
	}
	t.Fatalf("player %s missing from state", playerID)
	return false
}

func playerAFK(t *testing.T, state *room.Snapshot, playerID string) bool {
	t.Helper()

	for _, player := range state.Players {
		if player.ID == playerID {
			return player.AFK
		}
	}
	t.Fatalf("player %s missing from state", playerID)
	return false
}

func playerRole(t *testing.T, state *room.Snapshot, playerID string) room.Role {
	t.Helper()

	for _, player := range state.Players {
		if player.ID == playerID {
			return player.Role
		}
	}
	t.Fatalf("player %s missing from state", playerID)
	return ""
}

func firstClientIDExcept(clients []*testClient, excludedID string) string {
	for _, client := range clients {
		if client.id != excludedID {
			return client.id
		}
	}
	return ""
}

func closeClients(clients []*testClient) {
	for _, client := range clients {
		_ = client.conn.Close()
	}
}
