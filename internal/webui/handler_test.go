package webui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesGameClient(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	NewHandler().ServeHTTP(response, request)

	body, err := io.ReadAll(response.Result().Body)
	if err != nil {
		t.Fatalf("read game client: %v", err)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(string(body), "Midnight Council") {
		t.Fatal("game client does not contain product title")
	}
	if got := response.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("missing Content-Security-Policy header")
	}
}

func TestHandlerServesStaticAssets(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{path: "/app.css", contentType: "text/css"},
		{path: "/app.js", contentType: "text/javascript"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			NewHandler().ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if got := response.Header().Get("Content-Type"); !strings.Contains(got, test.contentType) {
				t.Fatalf("content type = %q, want %q", got, test.contentType)
			}
		})
	}
}

func TestHandlerRejectsUnsupportedMethod(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow = %q, want GET, HEAD", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestHandlerReturnsNotFoundForUnknownAsset(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestGameClientRendersServerDeadlineCountdown(t *testing.T) {
	index, err := assets.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	app, err := assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app: %v", err)
	}

	if !strings.Contains(string(index), `id="phase-countdown"`) {
		t.Fatal("game client is missing phase countdown element")
	}
	for _, field := range []string{"phase_deadline", "server_time"} {
		if !strings.Contains(string(app), field) {
			t.Fatalf("game client does not consume %s", field)
		}
	}
}

func TestGameClientTranslatesChatModerationErrors(t *testing.T) {
	app, err := assets.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app: %v", err)
	}

	for _, message := range []string{
		"chat message rejected by moderation",
		"chat moderation unavailable",
	} {
		if !strings.Contains(string(app), message) {
			t.Fatalf("game client does not translate %q", message)
		}
	}
}
