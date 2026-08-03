package ws

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	DefaultConnectionsPerSecond   float64 = 2
	DefaultConnectionBurst        int     = 10
	DefaultMaxConnectionsPerIP    int     = 20
	DefaultRoomCreationsPerSecond float64 = 0.2
	DefaultRoomCreationBurst      int     = 5
	admissionEntryTTL                     = 10 * time.Minute
)

var (
	ErrConnectionRateLimited   = errors.New("connection rate limit exceeded; retry later")
	ErrConnectionLimitReached  = errors.New("connection limit for this address has been reached")
	ErrRoomCreationRateLimited = errors.New("room creation rate limit exceeded; retry later")
)

type ConnectionLimits struct {
	Connections RateLimit
	MaxPerIP    int
	RoomCreates RateLimit
}

func DefaultConnectionLimits() ConnectionLimits {
	return ConnectionLimits{
		Connections: RateLimit{
			EventsPerSecond: DefaultConnectionsPerSecond,
			Burst:           DefaultConnectionBurst,
		},
		MaxPerIP: DefaultMaxConnectionsPerIP,
		RoomCreates: RateLimit{
			EventsPerSecond: DefaultRoomCreationsPerSecond,
			Burst:           DefaultRoomCreationBurst,
		},
	}
}

func (l ConnectionLimits) Validate() error {
	if err := l.Connections.validate("connection"); err != nil {
		return err
	}
	if l.MaxPerIP <= 0 {
		return errors.New("maximum connections per IP must be positive")
	}
	return l.RoomCreates.validate("room creation")
}

type admissionEntry struct {
	connections      int
	connectionBucket tokenBucket
	roomCreateBucket tokenBucket
	lastSeen         time.Time
}

type admissionLimiter struct {
	mu      sync.Mutex
	limits  ConnectionLimits
	entries map[string]*admissionEntry
}

func newAdmissionLimiter(limits ConnectionLimits) *admissionLimiter {
	return &admissionLimiter{
		limits:  limits,
		entries: make(map[string]*admissionEntry),
	}
}

func (l *admissionLimiter) acquire(clientIP string, creatingRoom bool, now time.Time) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.removeExpired(now)

	entry, ok := l.entries[clientIP]
	if !ok {
		entry = &admissionEntry{
			connectionBucket: newTokenBucket(l.limits.Connections, now),
			roomCreateBucket: newTokenBucket(l.limits.RoomCreates, now),
		}
		l.entries[clientIP] = entry
	}
	entry.lastSeen = now
	if !entry.connectionBucket.allow(now) {
		return nil, ErrConnectionRateLimited
	}
	if entry.connections >= l.limits.MaxPerIP {
		return nil, ErrConnectionLimitReached
	}
	if creatingRoom && !entry.roomCreateBucket.allow(now) {
		return nil, ErrRoomCreationRateLimited
	}
	entry.connections++

	var once sync.Once
	return func() {
		once.Do(func() {
			l.release(clientIP, time.Now())
		})
	}, nil
}

func (l *admissionLimiter) release(clientIP string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[clientIP]
	if !ok {
		return
	}
	if entry.connections > 0 {
		entry.connections--
	}
	entry.lastSeen = now
}

func (l *admissionLimiter) removeExpired(now time.Time) {
	for clientIP, entry := range l.entries {
		if entry.connections == 0 && now.Sub(entry.lastSeen) >= admissionEntryTTL {
			delete(l.entries, clientIP)
		}
	}
}

func remoteIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}
