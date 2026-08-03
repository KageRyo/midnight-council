package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
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
	hub              *room.Hub
	upgrader         websocket.Upgrader
	rateLimits       EventRateLimits
	connectionLimits ConnectionLimits
	admission        *admissionLimiter
	origins          originPolicy
	chatPolicy       moderation.ChatPolicy
}

type HandlerOption func(*Handler)

func WithEventRateLimits(limits EventRateLimits) HandlerOption {
	return func(h *Handler) {
		h.rateLimits = limits
	}
}

func WithConnectionLimits(limits ConnectionLimits) HandlerOption {
	return func(h *Handler) {
		h.connectionLimits = limits
	}
}

func WithAllowedOrigins(origins []string) HandlerOption {
	return func(h *Handler) {
		policy, err := newOriginPolicy(origins)
		if err != nil {
			panic(err)
		}
		h.origins = policy
	}
}

func WithChatPolicy(policy moderation.ChatPolicy) HandlerOption {
	return func(h *Handler) {
		h.chatPolicy = policy
	}
}

func NewHandler(hub *room.Hub, options ...HandlerOption) *Handler {
	handler := &Handler{
		hub:              hub,
		rateLimits:       DefaultEventRateLimits(),
		connectionLimits: DefaultConnectionLimits(),
		chatPolicy:       moderation.AllowAllChat{},
	}
	for _, option := range options {
		option(handler)
	}
	if err := handler.rateLimits.Validate(); err != nil {
		panic(err)
	}
	if err := handler.connectionLimits.Validate(); err != nil {
		panic(err)
	}
	if handler.chatPolicy == nil {
		panic("chat moderation policy is required")
	}
	handler.admission = newAdmissionLimiter(handler.connectionLimits)
	handler.upgrader.CheckOrigin = handler.origins.allows
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
	spectator := false
	if rawSpectator := strings.TrimSpace(r.URL.Query().Get("spectator")); rawSpectator != "" {
		spectator, err = strconv.ParseBool(rawSpectator)
		if err != nil {
			http.Error(w, "spectator query param must be a boolean", http.StatusBadRequest)
			return
		}
	}
	playerID, playerName, err = room.NormalizeParticipantIdentity(playerID, playerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if reconnectToken != "" && !reconnectTokenPattern.MatchString(reconnectToken) {
		http.Error(w, "reconnect token is invalid", http.StatusBadRequest)
		return
	}
	creatingRoom := !h.hub.RoomExists(roomID)
	if creatingRoom && !h.hub.CanCreateRoom() {
		http.Error(w, room.ErrRoomLimitReached.Error(), http.StatusTooManyRequests)
		return
	}
	release, err := h.admission.acquire(remoteIP(r), creatingRoom, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	defer release()

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	actor, err := h.hub.GetOrCreate(roomID)
	if err != nil {
		writeTerminalError(conn, err.Error())
		return
	}
	ctx := r.Context()

	_, err = actor.Dispatch(ctx, room.Event{
		Type:           room.EventJoin,
		PlayerID:       playerID,
		PlayerName:     playerName,
		ReconnectToken: reconnectToken,
		Spectator:      spectator,
	})
	if err != nil {
		writeTerminalError(conn, err.Error())
		return
	}
	events, subscriptionID, unsubscribe, err := actor.SubscribeConnection(ctx, playerID)
	if err != nil {
		writeTerminalError(conn, err.Error())
		return
	}

	done := make(chan struct{})
	errors := make(chan room.Envelope, 8)
	go writePump(conn, events, errors, done)
	limiter := newConnectionRateLimiter(h.rateLimits, time.Now())
	permanent := readPump(ctx, actor, subscriptionID, roomID, playerID, playerName, conn, errors, limiter, h.chatPolicy)
	close(done)
	unsubscribe(permanent)
}

func readPump(
	ctx context.Context,
	actor *room.Actor,
	subscriptionID string,
	roomID string,
	playerID string,
	playerName string,
	conn *websocket.Conn,
	outgoing chan<- room.Envelope,
	limiter *connectionRateLimiter,
	chatPolicy moderation.ChatPolicy,
) bool {
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			var closeError *websocket.CloseError
			return errors.As(err, &closeError) &&
				closeError.Code == websocket.CloseNormalClosure &&
				closeError.Text == "player left"
		}
		if messageType != websocket.TextMessage {
			if !sendError(outgoing, "client events must be text JSON messages") {
				return false
			}
			continue
		}

		incoming, err := protocol.DecodeClientEvent(bytes.NewReader(payload))
		if err != nil {
			if !sendError(outgoing, err.Error()) {
				return false
			}
			continue
		}
		sequence := incoming.SequenceValue()

		if !limiter.allow(incoming.Type, time.Now()) {
			message := rateLimitError(incoming.Type)
			if !sendError(outgoing, message) || !sendAck(outgoing, sequence, "rejected") {
				return false
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
				if !sendError(outgoing, publicError) || !sendAck(outgoing, sequence, "rejected") {
					return false
				}
				continue
			}
			event.Message = message
		}
		if _, err := actor.DispatchFrom(ctx, subscriptionID, event); err != nil {
			if errors.Is(err, room.ErrDuplicateClientEvent) {
				if !sendAck(outgoing, sequence, "duplicate") {
					return false
				}
				continue
			}
			if errors.Is(err, room.ErrConnectionReplaced) {
				continue
			}
			if !sendError(outgoing, err.Error()) || !sendAck(outgoing, sequence, "rejected") {
				return false
			}
			continue
		}
		if !sendAck(outgoing, sequence, "applied") {
			return false
		}
	}
}

func sendAck(outgoing chan<- room.Envelope, sequence uint64, status string) bool {
	if sequence == 0 {
		return true
	}
	select {
	case outgoing <- room.Envelope{
		Type: "ack",
		Ack: &room.EventAck{
			Sequence: sequence,
			Status:   status,
		},
	}:
		return true
	default:
		return false
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
	defer conn.Close()

	for {
		select {
		case envelope, ok := <-events:
			if !ok {
				_ = conn.Close()
				return
			}
			if err := writeEnvelope(conn, envelope); err != nil {
				return
			}
			if terminalEnvelope(envelope) {
				writeCloseControl(conn, envelope.Error)
				return
			}
		case envelope := <-errors:
			if err := writeEnvelope(conn, envelope); err != nil {
				return
			}
			if terminalEnvelope(envelope) {
				writeCloseControl(conn, envelope.Error)
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

func writeTerminalError(conn *websocket.Conn, message string) {
	_ = writeEnvelope(conn, room.Envelope{Type: "error", Error: message})
	writeCloseControl(conn, message)
}

func terminalEnvelope(envelope room.Envelope) bool {
	return envelope.Type == "error" &&
		(envelope.Error == room.ErrKicked.Error() || envelope.Error == room.ErrConnectionReplaced.Error())
}

func writeCloseControl(conn *websocket.Conn, message string) {
	closeReason := message
	if len([]byte(closeReason)) > 120 {
		closeReason = "connection rejected"
	}
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, closeReason),
		time.Now().Add(writeWait),
	)
}

func roomIDFromPath(path string) (string, error) {
	roomID := strings.TrimPrefix(path, "/ws/rooms/")
	roomID = strings.Trim(roomID, "/")
	if roomID == "" || roomID == path || !roomIDPattern.MatchString(roomID) {
		return "", errors.New("room id is required")
	}
	return roomID, nil
}

var (
	roomIDPattern         = regexp.MustCompile(`^[A-Za-z0-9_-]{2,48}$`)
	reconnectTokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)
