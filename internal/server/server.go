package server

import (
	"context"
	"net/http"
	"time"

	"github.com/yxorp/internal/config"
)

type Server struct {
	httpServer *http.Server
	certFile   string
	keyFile    string
}

func NewServer(cfg config.ServerConfig, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  120 * time.Second, // Requirement: 120s
		},
		certFile: cfg.CertFile,
		keyFile:  cfg.KeyFile,
	}
}

func (s *Server) Start() error {
	if s.certFile != "" && s.keyFile != "" {
		return s.httpServer.ListenAndServeTLS(s.certFile, s.keyFile)
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
