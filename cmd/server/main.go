package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"midnight-council/internal/room"
	"midnight-council/internal/webui"
	"midnight-council/internal/ws"
)

func main() {
	addr, err := listenAddress()
	if err != nil {
		log.Fatalf("invalid server address configuration: %v", err)
	}
	roomIdleTimeout := getenvDuration("ROOM_IDLE_TIMEOUT", room.DefaultRoomIdleTimeout)
	phaseDurations := room.PhaseDurations{
		Night:         getenvDuration("NIGHT_DURATION", room.DefaultNightDuration),
		DayDiscussion: getenvDuration("DAY_DISCUSSION_DURATION", room.DefaultDayDiscussionDuration),
		DayVoting:     getenvDuration("DAY_VOTING_DURATION", room.DefaultDayVotingDuration),
		LastWords:     getenvDuration("LAST_WORDS_DURATION", room.DefaultLastWordsDuration),
	}
	if err := phaseDurations.Validate(); err != nil {
		log.Fatalf("invalid phase duration configuration: %v", err)
	}
	rateLimits := ws.EventRateLimits{
		Chat: ws.RateLimit{
			EventsPerSecond: getenvFloat64("WS_CHAT_EVENTS_PER_SECOND", ws.DefaultChatEventsPerSecond),
			Burst:           getenvInt("WS_CHAT_BURST", ws.DefaultChatBurst),
		},
		Game: ws.RateLimit{
			EventsPerSecond: getenvFloat64("WS_GAME_EVENTS_PER_SECOND", ws.DefaultGameEventsPerSecond),
			Burst:           getenvInt("WS_GAME_BURST", ws.DefaultGameBurst),
		},
	}
	if err := rateLimits.Validate(); err != nil {
		log.Fatalf("invalid WebSocket rate limit configuration: %v", err)
	}
	connectionLimits := ws.ConnectionLimits{
		Connections: ws.RateLimit{
			EventsPerSecond: getenvFloat64("WS_CONNECTIONS_PER_SECOND", ws.DefaultConnectionsPerSecond),
			Burst:           getenvInt("WS_CONNECTION_BURST", ws.DefaultConnectionBurst),
		},
		MaxPerIP: getenvInt("WS_MAX_CONNECTIONS_PER_IP", ws.DefaultMaxConnectionsPerIP),
		RoomCreates: ws.RateLimit{
			EventsPerSecond: getenvFloat64("WS_ROOM_CREATIONS_PER_SECOND", ws.DefaultRoomCreationsPerSecond),
			Burst:           getenvInt("WS_ROOM_CREATION_BURST", ws.DefaultRoomCreationBurst),
		},
	}
	if err := connectionLimits.Validate(); err != nil {
		log.Fatalf("invalid WebSocket connection limit configuration: %v", err)
	}
	allowedOrigins, err := ws.ParseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))
	if err != nil {
		log.Fatalf("invalid ALLOWED_ORIGINS configuration: %v", err)
	}
	trustedProxies, err := ws.ParseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		log.Fatalf("invalid TRUSTED_PROXIES configuration: %v", err)
	}
	maxRooms := getenvInt("MAX_ROOMS", room.DefaultMaxRooms)
	if maxRooms <= 0 {
		log.Fatal("MAX_ROOMS must be positive")
	}
	maxSpectators := getenvInt("MAX_SPECTATORS_PER_ROOM", room.DefaultMaxSpectators)
	if maxSpectators <= 0 {
		log.Fatal("MAX_SPECTATORS_PER_ROOM must be positive")
	}

	hub := room.NewHub(
		room.WithRoomIdleTimeout(roomIdleTimeout),
		room.WithPhaseDurations(phaseDurations),
		room.WithMaxRooms(maxRooms),
		room.WithMaxSpectators(maxSpectators),
	)
	handler := ws.NewHandler(
		hub,
		ws.WithEventRateLimits(rateLimits),
		ws.WithConnectionLimits(connectionLimits),
		ws.WithAllowedOrigins(allowedOrigins),
		ws.WithTrustedProxies(trustedProxies),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/ws/rooms/", handler)
	mux.Handle("/", webui.NewHandler())

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf(
			"midnight-council game server listening on %s with room idle timeout %s, room cap=%d, spectator cap=%d, allowed origins=%d, trusted proxy networks=%d, phase durations night=%s discussion=%s voting=%s last_words=%s, and WebSocket rate limits chat=%g/s burst=%d game=%g/s burst=%d connection=%g/s burst=%d per_ip=%d room_create=%g/s burst=%d",
			addr,
			roomIdleTimeout,
			maxRooms,
			maxSpectators,
			len(allowedOrigins),
			len(trustedProxies),
			phaseDurations.Night,
			phaseDurations.DayDiscussion,
			phaseDurations.DayVoting,
			phaseDurations.LastWords,
			rateLimits.Chat.EventsPerSecond,
			rateLimits.Chat.Burst,
			rateLimits.Game.EventsPerSecond,
			rateLimits.Game.Burst,
			connectionLimits.Connections.EventsPerSecond,
			connectionLimits.Connections.Burst,
			connectionLimits.MaxPerIP,
			connectionLimits.RoomCreates.EventsPerSecond,
			connectionLimits.RoomCreates.Burst,
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
}

func listenAddress() (string, error) {
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr, nil
	}
	port := getenv("PORT", "8080")
	parsed, err := strconv.Atoi(port)
	if err != nil || parsed < 1 || parsed > 65535 {
		return "", fmt.Errorf("PORT must be a number from 1 to 65535, got %q", port)
	}
	return ":" + port, nil
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf("invalid %s duration %q: %v", key, value, err)
	}
	return duration
}

func getenvFloat64(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Fatalf("invalid %s value %q: %v", key, value, err)
	}
	return parsed
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("invalid %s value %q: %v", key, value, err)
	}
	return parsed
}
