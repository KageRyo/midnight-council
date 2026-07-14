package ws

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"midnight-council/internal/room"
)

type testClient struct {
	id   string
	conn *websocket.Conn
}

type wireEnvelope struct {
	Type    string                  `json:"type"`
	State   *room.Snapshot          `json:"state,omitempty"`
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
	return connectClientWithToken(t, serverURL, playerID, playerName, "")
}

func connectClientWithToken(t *testing.T, serverURL, playerID, playerName, reconnectToken string) *testClient {
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
