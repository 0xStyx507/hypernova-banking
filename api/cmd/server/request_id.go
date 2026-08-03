package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type requestIDContextKey struct{}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// requestLogMiddleware emits one structured access record per request. It
// intentionally records the route path without query strings or request body,
// so diagnostics never capture tokens, PINs or financial payloads.
func requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		writer := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		if r.URL.Path == healthPath || r.URL.Path == readinessPath || r.URL.Path == versionedHealthPath {
			return
		}
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}
		slog.Default().Log(r.Context(), level, "http request", "method", r.Method, "path", r.URL.Path, "status", status, "duration_ms", time.Since(startedAt).Milliseconds(), "request_id", requestID(r))
	})
}

// requestIDMiddleware gives every response a stable correlation identifier.
// A caller-supplied identifier is accepted only when it is a valid UUID, which
// keeps logs and support references predictable without allowing header/control
// characters into downstream systems.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if _, err := uuid.Parse(requestID); err != nil {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	value, _ := r.Context().Value(requestIDContextKey{}).(string)
	return value
}
