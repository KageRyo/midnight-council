package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	conn *websocket.Conn
	done chan struct{}
}

type result struct {
	duration time.Duration
	err      error
}

func main() {
	target := flag.String("target", "ws://127.0.0.1:8080", "WebSocket server origin")
	roomPrefix := flag.String("room-prefix", "load", "prefix for generated room identifiers")
	rooms := flag.Int("rooms", 10, "number of rooms to create")
	players := flag.Int("players", 10, "players to connect to each room")
	workers := flag.Int("workers", 20, "maximum simultaneous connection attempts")
	hold := flag.Duration("hold", 15*time.Second, "how long to keep successful connections open")
	exercise := flag.Bool("exercise-events", true, "send one presence and chat event from every connected client")
	requireAll := flag.Bool("require-all", true, "exit unsuccessfully if any connection fails")
	flag.Parse()

	if *rooms <= 0 || *players <= 0 || *workers <= 0 || *hold < 0 {
		log.Fatal("rooms, players, and workers must be positive; hold must not be negative")
	}
	serverURL, err := url.Parse(*target)
	if err != nil || (serverURL.Scheme != "ws" && serverURL.Scheme != "wss") || serverURL.Host == "" {
		log.Fatalf("target must be a ws:// or wss:// origin: %q", *target)
	}

	total := *rooms * *players
	requests := make(chan int)
	results := make(chan result, total)
	clients := make(chan *client, total)
	var group sync.WaitGroup
	for worker := 0; worker < *workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range requests {
				roomNumber := index / *players
				playerNumber := index % *players
				connected, duration, connectErr := connect(serverURL, *roomPrefix, roomNumber, playerNumber)
				results <- result{duration: duration, err: connectErr}
				if connected != nil {
					clients <- connected
				}
			}
		}()
	}
	go func() {
		for index := 0; index < total; index++ {
			requests <- index
		}
		close(requests)
		group.Wait()
		close(results)
		close(clients)
	}()

	connected := make([]*client, 0, total)
	for connectedClient := range clients {
		connected = append(connected, connectedClient)
	}

	var failed atomic.Int64
	latencies := make([]time.Duration, 0, total)
	for connectResult := range results {
		if connectResult.err != nil {
			failed.Add(1)
			log.Printf("connection failed: %v", connectResult.err)
			continue
		}
		latencies = append(latencies, connectResult.duration)
	}

	log.Printf("connected %d/%d clients; p50=%s p95=%s", len(connected), total, percentile(latencies, 0.50), percentile(latencies, 0.95))
	if *exercise {
		exerciseEvents(connected)
	}
	if *hold > 0 {
		time.Sleep(*hold)
	}
	for _, connectedClient := range connected {
		close(connectedClient.done)
		_ = connectedClient.conn.Close()
	}
	log.Printf("closed %d clients; failures=%d", len(connected), failed.Load())
	if *requireAll && failed.Load() > 0 {
		os.Exit(1)
	}
}

func connect(serverURL *url.URL, roomPrefix string, roomNumber, playerNumber int) (*client, time.Duration, error) {
	endpoint := *serverURL
	endpoint.Path = fmt.Sprintf("/ws/rooms/%s-%03d", roomPrefix, roomNumber+1)
	query := endpoint.Query()
	query.Set("player_id", fmt.Sprintf("load-%03d-%03d", roomNumber+1, playerNumber+1))
	query.Set("name", fmt.Sprintf("Load %03d/%03d", roomNumber+1, playerNumber+1))
	endpoint.RawQuery = query.Encode()

	started := time.Now()
	connection, _, err := websocket.DefaultDialer.Dial(endpoint.String(), nil)
	if err != nil {
		return nil, time.Since(started), err
	}
	connected := &client{conn: connection, done: make(chan struct{})}
	go func() {
		defer connection.Close()
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
			select {
			case <-connected.done:
				return
			default:
			}
		}
	}()
	return connected, time.Since(started), nil
}

func exerciseEvents(clients []*client) {
	var group sync.WaitGroup
	for _, connectedClient := range clients {
		group.Add(1)
		go func(connectedClient *client) {
			defer group.Done()
			_ = connectedClient.conn.WriteJSON(map[string]any{"type": "presence", "afk": false})
			_ = connectedClient.conn.WriteJSON(map[string]any{"type": "chat", "message": "load check"})
		}(connectedClient)
	}
	group.Wait()
}

func percentile(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	index := int(float64(len(sorted)-1) * percentile)
	return sorted[index]
}
