package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"midnight-council/internal/moderation"
	"midnight-council/internal/protocol"
	"midnight-council/internal/room"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 45 * time.Second
)

type Handler struct {
	hub        *room.Hub
	upgrader   websocket.Upgrader
	rateLimits EventRateLimits
	chatPolicy moderation.ChatPolicy
}

type HandlerOption func(*Handler)

func WithEventRateLimits(limits EventRateLimits) HandlerOption {
	return func(h *Handler) {
		h.rateLimits = limits
	}
}

func WithChatPolicy(policy moderation.ChatPolicy) HandlerOption {
	return func(h *Handler) {
		h.chatPolicy = policy
	}
}

func NewHandler(hub *room.Hub, options ...HandlerOption) *Handler {
	handler := &Handler{
		hub:        hub,
		rateLimits: DefaultEventRateLimits(),
		chatPolicy: moderation.AllowAllChat{},
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
	for _, option := range options {
		option(handler)
	}
	if err := handler.rateLimits.Validate(); err != nil {
		panic(err)
	}
	if handler.chatPolicy == nil {
		panic("chat moderation policy is required")
	}
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID, err := roomIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	playerID := strings.TrimSpace(r.URL.Query().Get("player_id"))
	playerName := strings.TrimSpace(r.URL.Query().Get("name"))
	reconnectToken := strings.TrimSpace(r.URL.Query().Get("reconnect_token"))
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

	_, err = actor.Dispatch(ctx, room.Event{
		Type:           room.EventJoin,
		PlayerID:       playerID,
		PlayerName:     playerName,
		ReconnectToken: reconnectToken,
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

	events, unsubscribe, err := actor.Subscribe(ctx, playerID)
	if err != nil {
		_ = writeEnvelope(conn, room.Envelope{Type: "error", Error: err.Error()})
		return
	}
	defer unsubscribe()

	done := make(chan struct{})
	errors := make(chan room.Envelope, 8)
	go writePump(conn, events, errors, done)
	limiter := newConnectionRateLimiter(h.rateLimits, time.Now())
	readPump(ctx, actor, roomID, playerID, playerName, conn, errors, limiter, h.chatPolicy)
	close(done)
}

func readPump(
	ctx context.Context,
	actor *room.Actor,
	roomID string,
	playerID string,
	playerName string,
	conn *websocket.Conn,
	errors chan<- room.Envelope,
	limiter *connectionRateLimiter,
	chatPolicy moderation.ChatPolicy,
) {
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			if !sendError(errors, "client events must be text JSON messages") {
				return
			}
			continue
		}

		incoming, err := protocol.DecodeClientEvent(bytes.NewReader(payload))
		if err != nil {
			if !sendError(errors, err.Error()) {
				return
			}
			continue
		}

		if !limiter.allow(incoming.Type, time.Now()) {
			if !sendError(errors, rateLimitError(incoming.Type)) {
				return
			}
			continue
		}

		event := incoming.RoomEvent(playerID, playerName)
		if event.Type == room.EventChat {
			message, publicError, moderationErr := moderateChat(ctx, chatPolicy, moderation.ChatRequest{
				RoomID:     roomID,
				PlayerID:   playerID,
				PlayerName: playerName,
				Message:    event.Message,
			})
			if moderationErr != nil {
				log.Printf("chat moderation failed for room %q player %q: %v", roomID, playerID, moderationErr)
			}
			if publicError != "" {
				if !sendError(errors, publicError) {
					return
				}
				continue
			}
			event.Message = message
		}
		if _, err := actor.Dispatch(ctx, event); err != nil {
			if !sendError(errors, err.Error()) {
				return
			}
		}
	}
}

func sendError(errors chan<- room.Envelope, message string) bool {
	select {
	case errors <- room.Envelope{Type: "error", Error: message}:
		return true
	default:
		return false
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
