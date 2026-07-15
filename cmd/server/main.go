package main

import (
	"context"
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
	addr := getenv("ADDR", ":8080")
	roomIdleTimeout := getenvDuration("ROOM_IDLE_TIMEOUT", room.DefaultRoomIdleTimeout)
	phaseDurations := room.PhaseDurations{
		Night:         getenvDuration("NIGHT_DURATION", room.DefaultNightDuration),
		DayDiscussion: getenvDuration("DAY_DISCUSSION_DURATION", room.DefaultDayDiscussionDuration),
		DayVoting:     getenvDuration("DAY_VOTING_DURATION", room.DefaultDayVotingDuration),
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

	hub := room.NewHub(
		room.WithRoomIdleTimeout(roomIdleTimeout),
		room.WithPhaseDurations(phaseDurations),
	)
	handler := ws.NewHandler(hub, ws.WithEventRateLimits(rateLimits))

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
			"midnight-council game server listening on %s with room idle timeout %s, phase durations night=%s discussion=%s voting=%s, and WebSocket rate limits chat=%g/s burst=%d game=%g/s burst=%d",
			addr,
			roomIdleTimeout,
			phaseDurations.Night,
			phaseDurations.DayDiscussion,
			phaseDurations.DayVoting,
			rateLimits.Chat.EventsPerSecond,
			rateLimits.Chat.Burst,
			rateLimits.Game.EventsPerSecond,
			rateLimits.Game.Burst,
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
