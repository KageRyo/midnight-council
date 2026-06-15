package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"midnight-council/internal/room"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 45 * time.Second
)

type Handler struct {
	hub      *room.Hub
	upgrader websocket.Upgrader
}

type clientEvent struct {
	Type    room.EventType `json:"type"`
	Ready   bool           `json:"ready"`
	Message string         `json:"message"`
}

func NewHandler(hub *room.Hub) *Handler {
	return &Handler{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID, err := roomIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	playerID := strings.TrimSpace(r.URL.Query().Get("player_id"))
	playerName := strings.TrimSpace(r.URL.Query().Get("name"))
	if playerID == "" || playerName == "" {
		http.Error(w, "player_id and name query params are required", http.StatusBadRequest)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	actor := h.hub.GetOrCreate(roomID)
	ctx := r.Context()

	events, unsubscribe, err := actor.Subscribe(ctx, playerID)
	if err != nil {
		_ = writeEnvelope(conn, room.Envelope{Type: "error", Error: err.Error()})
		return
	}
	defer unsubscribe()

	_, err = actor.Dispatch(ctx, room.Event{
		Type:       room.EventJoin,
		PlayerID:   playerID,
		PlayerName: playerName,
	})
	if err != nil {
		_ = writeEnvelope(conn, room.Envelope{Type: "error", Error: err.Error()})
		return
	}
	defer func() {
		leaveCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = actor.Dispatch(leaveCtx, room.Event{
			Type:     room.EventLeave,
			PlayerID: playerID,
		})
	}()

	done := make(chan struct{})
	errors := make(chan room.Envelope, 8)
	go writePump(conn, events, errors, done)
	readPump(ctx, actor, playerID, playerName, conn, errors)
	close(done)
}

func readPump(ctx context.Context, actor *room.Actor, playerID, playerName string, conn *websocket.Conn, errors chan<- room.Envelope) {
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		var incoming clientEvent
		if err := conn.ReadJSON(&incoming); err != nil {
			return
		}

		event := room.Event{
			Type:       incoming.Type,
			PlayerID:   playerID,
			PlayerName: playerName,
			Ready:      incoming.Ready,
			Message:    incoming.Message,
		}

		if _, err := actor.Dispatch(ctx, event); err != nil {
			select {
			case errors <- room.Envelope{Type: "error", Error: err.Error()}:
			default:
				return
			}
		}
	}
}

func writePump(conn *websocket.Conn, events <-chan room.Envelope, errors <-chan room.Envelope, done <-chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case envelope, ok := <-events:
			if !ok {
				return
			}
			if err := writeEnvelope(conn, envelope); err != nil {
				return
			}
		case envelope := <-errors:
			if err := writeEnvelope(conn, envelope); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func writeEnvelope(conn *websocket.Conn, envelope room.Envelope) error {
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	writer, err := conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}
	defer writer.Close()
	return json.NewEncoder(writer).Encode(envelope)
}

func roomIDFromPath(path string) (string, error) {
	roomID := strings.TrimPrefix(path, "/ws/rooms/")
	roomID = strings.Trim(roomID, "/")
	if roomID == "" || roomID == path {
		return "", errors.New("room id is required")
	}
	return roomID, nil
}
