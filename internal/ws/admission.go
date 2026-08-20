package ws

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
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

// ParseTrustedProxies parses a comma-separated list of proxy IP addresses or
// CIDR prefixes. An empty value disables forwarded-address processing.
func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("trusted proxy list contains an empty entry")
		}

		if prefix, err := netip.ParsePrefix(part); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}

		address, err := netip.ParseAddr(part)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: must be an IP address or CIDR prefix", part)
		}
		address = address.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(address, address.BitLen()))
	}

	return prefixes, nil
}

func remoteIP(request *http.Request, configured ...[]netip.Prefix) string {
	var trustedProxies []netip.Prefix
	if len(configured) > 0 {
		trustedProxies = configured[0]
	}
	peer, ok := requestPeerIP(request.RemoteAddr)
	if !ok {
		return request.RemoteAddr
	}
	if !containsTrustedProxy(trustedProxies, peer) {
		return peer.String()
	}

	forwarded := forwardedIPs(request)
	if len(forwarded) == 0 {
		return peer.String()
	}

	chain := append(forwarded, peer)
	for index := len(chain) - 1; index >= 0; index-- {
		if !containsTrustedProxy(trustedProxies, chain[index]) {
			return chain[index].String()
		}
	}

	// If every hop is in a configured proxy network, trust the left-most
	// address supplied by that proxy chain as the original client address.
	return chain[0].String()
}

func requestPeerIP(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func containsTrustedProxy(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func forwardedIPs(request *http.Request) []netip.Addr {
	if values := request.Header.Values("X-Forwarded-For"); len(values) > 0 {
		return parseForwardedIPList(values)
	}
	if values := request.Header.Values("Forwarded"); len(values) > 0 {
		return parseForwardedHeader(values)
	}
	return nil
}

func parseForwardedIPList(values []string) []netip.Addr {
	var addresses []netip.Addr
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			address, ok := parseForwardedAddress(item)
			if !ok {
				return nil
			}
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func parseForwardedHeader(values []string) []netip.Addr {
	var addresses []netip.Addr
	for _, value := range values {
		for _, element := range strings.Split(value, ",") {
			var address netip.Addr
			found := false
			for _, parameter := range strings.Split(element, ";") {
				name, rawValue, ok := strings.Cut(strings.TrimSpace(parameter), "=")
				if !ok || !strings.EqualFold(name, "for") {
					continue
				}
				if found {
					return nil
				}
				var valid bool
				address, valid = parseForwardedAddress(rawValue)
				if !valid {
					return nil
				}
				found = true
			}
			if !found {
				return nil
			}
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func parseForwardedAddress(value string) (netip.Addr, bool) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if address, err := netip.ParseAddr(value); err == nil {
		return address.Unmap(), true
	}
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		return addressPort.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}
