package apierr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteForLogging(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(prev)

	// A canceled request isn't an error worth logging.
	r := httptest.NewRequest("GET", "/api/admin/v1/dashboard", nil)
	w := httptest.NewRecorder()
	WriteFor(w, r, fmt.Errorf("query: %w", context.Canceled))
	if w.Code != 499 || logs.Len() != 0 {
		t.Fatalf("canceled: %d, logs %q", w.Code, logs.String())
	}
	// Unexpected errors are logged with the request, never sent to clients.
	w = httptest.NewRecorder()
	WriteFor(w, r, errors.New("disk on fire"))
	if w.Code != 500 || strings.Contains(w.Body.String(), "disk") {
		t.Fatalf("internal: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(logs.String(), "path=/api/admin/v1/dashboard") || !strings.Contains(logs.String(), "disk on fire") {
		t.Fatalf("log lacks context: %q", logs.String())
	}
	// API errors pass through unchanged.
	w = httptest.NewRecorder()
	WriteFor(w, r, Forbidden("csrf_origin", "nope"))
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"csrf_origin"`) {
		t.Fatalf("api error: %d %s", w.Code, w.Body)
	}
}
