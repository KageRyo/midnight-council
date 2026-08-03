package ws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"midnight-council/internal/room"
)

func TestHandlerOriginPolicy(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub(), WithAllowedOrigins([]string{"https://game.example.com"})))
	defer server.Close()

	allowed := dialParticipantWithHeaders(t, server.URL, "allowed", "Allowed", http.Header{"Origin": []string{"https://game.example.com"}})
	defer allowed.Close()

	_, response, err := dialParticipant(server.URL, "rejected", "Rejected", http.Header{"Origin": []string{"https://evil.example.com"}})
	if err == nil {
		t.Fatal("disallowed origin connected")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed origin response = %#v, want HTTP 403", response)
	}
}

func TestHandlerRejectsInvalidJoinIdentifiers(t *testing.T) {
	handler := NewHandler(room.NewHub())
	request := httptest.NewRequest(http.MethodGet, "/ws/rooms/not/a-room?player_id=player&name=Player", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid room status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	request = httptest.NewRequest(http.MethodGet, "/ws/rooms/valid-room?player_id=player%2Fid&name=Player", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), room.ErrInvalidPlayerID.Error()) {
		t.Fatalf("invalid player response = %d %q", response.Code, response.Body.String())
	}
}

func TestHandlerLimitsConnectionsAndRoomCreation(t *testing.T) {
	limits := ConnectionLimits{
		Connections: RateLimit{EventsPerSecond: 100, Burst: 10},
		MaxPerIP:    1,
		RoomCreates: RateLimit{EventsPerSecond: 0.01, Burst: 1},
	}
	server := httptest.NewServer(NewHandler(room.NewHub(), WithConnectionLimits(limits)))
	defer server.Close()

	first := dialParticipantWithHeaders(t, server.URL, "first", "First", nil)
	defer first.Close()

	_, response, err := dialParticipant(server.URL, "second", "Second", nil)
	if err == nil {
		t.Fatal("second connection exceeded the concurrent per-IP limit")
	}
	if response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("connection limit response = %#v, want HTTP 429", response)
	}
	assertAdmissionError(t, response, ErrConnectionLimitReached)

	if err := first.Close(); err != nil {
		t.Fatalf("close first connection: %v", err)
	}
	eventuallyAdmission(t, func() bool {
		connection, candidateResponse, candidateErr := dialParticipantInRoom(server.URL, "another-room", "different", "Different", nil)
		if candidateErr == nil {
			_ = connection.Close()
			return false
		}
		response = candidateResponse
		if response == nil || response.StatusCode != http.StatusTooManyRequests {
			return false
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		return readErr == nil && strings.Contains(string(body), ErrRoomCreationRateLimited.Error())
	})
	if response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("room creation response = %#v, want HTTP 429", response)
	}
}

func TestHandlerRejectsNewRoomAtGlobalCapacity(t *testing.T) {
	server := httptest.NewServer(NewHandler(room.NewHub(room.WithMaxRooms(1))))
	defer server.Close()

	first := dialParticipantWithHeaders(t, server.URL, "first", "First", nil)
	defer first.Close()
	_, response, err := dialParticipantInRoom(server.URL, "second-room", "second", "Second", nil)
	if err == nil {
		t.Fatal("room beyond the global cap connected")
	}
	if response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("room limit response = %#v, want HTTP 429", response)
	}
	assertAdmissionError(t, response, room.ErrRoomLimitReached)
}

func TestConnectionLimitsValidate(t *testing.T) {
	limits := DefaultConnectionLimits()
	limits.MaxPerIP = 0
	if err := limits.Validate(); err == nil {
		t.Fatal("zero per-IP connection limit was accepted")
	}
}

func dialParticipantWithHeaders(t *testing.T, serverURL, playerID, playerName string, headers http.Header) *websocket.Conn {
	t.Helper()
	connection, _, err := dialParticipant(serverURL, playerID, playerName, headers)
	if err != nil {
		t.Fatalf("dial %s: %v", playerID, err)
	}
	return connection
}

func dialParticipant(serverURL, playerID, playerName string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	return dialParticipantInRoom(serverURL, "integration", playerID, playerName, headers)
}

func dialParticipantInRoom(serverURL, roomID, playerID, playerName string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, nil, err
	}
	parsed.Scheme = "ws"
	parsed.Path = "/ws/rooms/" + roomID
	query := parsed.Query()
	query.Set("player_id", playerID)
	query.Set("name", playerName)
	parsed.RawQuery = query.Encode()
	return websocket.DefaultDialer.Dial(parsed.String(), headers)
}

func eventuallyAdmission(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !condition() {
		t.Fatal("condition was not met before timeout")
	}
}

func assertAdmissionError(t *testing.T, response *http.Response, want error) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || !strings.Contains(string(body), want.Error()) {
		t.Fatalf("response body = %q, read error = %v, want %q", body, err, want)
	}
}
