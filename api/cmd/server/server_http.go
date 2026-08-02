package main

import (
	"context"
	"net/http"
	"time"
)

type serverLifecycle interface {
	Shutdown(context.Context) error
}

// newHTTPServer constructs the process boundary with explicit timeouts at
// every limit, preventing slow clients from holding connections indefinitely.
func newHTTPServer(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
