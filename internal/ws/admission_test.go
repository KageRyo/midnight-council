package ws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"midnight-council/internal/room"
)

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      []string
		wantError bool
	}{
		{name: "empty", value: "", want: nil},
		{name: "addresses and prefixes", value: "127.0.0.1, 10.0.0.0/8, 2001:db8::/32", want: []string{"127.0.0.1/32", "10.0.0.0/8", "2001:db8::/32"}},
		{name: "empty entry", value: "127.0.0.1,", wantError: true},
		{name: "invalid entry", value: "proxy.internal", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseTrustedProxies(test.value)
			if (err != nil) != test.wantError {
				t.Fatalf("ParseTrustedProxies() error = %v, want error = %t", err, test.wantError)
			}
			if err != nil {
				return
			}
			if len(got) != len(test.want) {
				t.Fatalf("ParseTrustedProxies() returned %d prefixes, want %d", len(got), len(test.want))
			}
			for index, prefix := range got {
				if prefix.String() != test.want[index] {
					t.Errorf("prefix %d = %q, want %q", index, prefix, test.want[index])
				}
			}
		})
	}
}

func TestRemoteIPUsesForwardedAddressOnlyFromTrustedPeer(t *testing.T) {
	proxies, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.RemoteAddr = "10.1.2.3:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")
	if got := remoteIP(request, proxies); got != "198.51.100.10" {
		t.Fatalf("remoteIP() = %q, want forwarded client IP", got)
	}

	request.RemoteAddr = "203.0.113.5:443"
	if got := remoteIP(request, proxies); got != "203.0.113.5" {
		t.Fatalf("remoteIP() = %q, want untrusted peer IP", got)
	}
}

func TestRemoteIPSelectsFirstUntrustedHopFromRight(t *testing.T) {
	proxies, err := ParseTrustedProxies("10.0.0.0/8, 172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.RemoteAddr = "10.1.2.3:443"
	request.Header.Set("X-Forwarded-For", "198.51.100.10, 172.16.2.4, 10.9.8.7")

	if got := remoteIP(request, proxies); got != "198.51.100.10" {
		t.Fatalf("remoteIP() = %q, want first untrusted hop", got)
	}
}

func TestRemoteIPIgnoresMalformedForwardedHeaders(t *testing.T) {
	proxies, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.RemoteAddr = "10.1.2.3:443"
	request.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := remoteIP(request, proxies); got != "10.1.2.3" {
		t.Fatalf("remoteIP() = %q, want direct peer after malformed header", got)
	}

	request.Header.Del("X-Forwarded-For")
	request.Header.Set("Forwarded", `for="[2001:db8::10]:1234";proto=https`)
	if got := remoteIP(request, proxies); got != "2001:db8::10" {
		t.Fatalf("remoteIP() = %q, want Forwarded client IP", got)
	}
}

func TestHandlerUsesForwardedClientIPForAdmission(t *testing.T) {
	proxies, err := ParseTrustedProxies("127.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	limits := ConnectionLimits{
		Connections: RateLimit{EventsPerSecond: 100, Burst: 10},
		MaxPerIP:    1,
		RoomCreates: RateLimit{EventsPerSecond: 100, Burst: 10},
	}
	server := httptest.NewServer(NewHandler(room.NewHub(), WithConnectionLimits(limits), WithTrustedProxies(proxies)))
	defer server.Close()

	first := dialParticipantWithHeaders(t, server.URL, "first", "First", http.Header{"X-Forwarded-For": []string{"198.51.100.10"}})
	defer first.Close()
	second := dialParticipantWithHeaders(t, server.URL, "second", "Second", http.Header{"X-Forwarded-For": []string{"198.51.100.11"}})
	defer second.Close()

	_, response, err := dialParticipant(server.URL, "same-client", "Same", http.Header{"X-Forwarded-For": []string{"198.51.100.10"}})
	if err == nil {
		t.Fatal("same forwarded client IP exceeded the concurrent per-IP limit")
	}
	if response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("same-client response = %#v, want HTTP 429", response)
	}
	body := readResponseBody(t, response)
	if !strings.Contains(body, ErrConnectionLimitReached.Error()) {
		t.Fatalf("same-client response body = %q, want %q", body, ErrConnectionLimitReached)
	}
}

func readResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
