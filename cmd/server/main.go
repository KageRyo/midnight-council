package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	hub := room.NewHub(
		room.WithRoomIdleTimeout(roomIdleTimeout),
		room.WithPhaseDurations(phaseDurations),
	)
	handler := ws.NewHandler(hub)

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
			"midnight-council game server listening on %s with room idle timeout %s and phase durations night=%s discussion=%s voting=%s",
			addr,
			roomIdleTimeout,
			phaseDurations.Night,
			phaseDurations.DayDiscussion,
			phaseDurations.DayVoting,
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
