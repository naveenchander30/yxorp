package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/yxorp/internal/config"
)

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := config.ServerConfig{
		Port:         "28080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := NewServer(cfg, handler)

	// Start in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start()
	}()

	// Allow server to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Failed to shutdown server: %v", err)
	}

	// Verify Start returned ErrServerClosed
	select {
	case err := <-errChan:
		if err != http.ErrServerClosed {
			t.Errorf("Expected ErrServerClosed, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Server start goroutine did not exit after shutdown")
	}
}
