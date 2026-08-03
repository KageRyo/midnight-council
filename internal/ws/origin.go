package ws

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type originPolicy struct {
	allowed map[string]struct{}
}

// ParseAllowedOrigins parses a comma-separated ALLOWED_ORIGINS configuration value.
func ParseAllowedOrigins(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin, err := normalizeOrigin(part)
		if err != nil {
			return nil, err
		}
		origins = append(origins, origin)
	}
	return origins, nil
}

func newOriginPolicy(origins []string) (originPolicy, error) {
	policy := originPolicy{allowed: make(map[string]struct{}, len(origins))}
	for _, origin := range origins {
		normalized, err := normalizeOrigin(origin)
		if err != nil {
			return originPolicy{}, err
		}
		policy.allowed[normalized] = struct{}{}
	}
	return policy, nil
}

func (p originPolicy) allows(request *http.Request) bool {
	rawOrigin := request.Header.Get("Origin")
	if rawOrigin == "" {
		return true
	}
	origin, err := normalizeOrigin(rawOrigin)
	if err != nil {
		return false
	}
	if len(p.allowed) > 0 {
		_, ok := p.allowed[origin]
		return ok
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, request.Host)
}

func normalizeOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid allowed origin %q", raw)
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("allowed origin %q must use http or https", raw)
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}
