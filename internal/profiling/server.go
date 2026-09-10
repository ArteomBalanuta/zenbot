package profiling

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	httppprof "net/http/pprof"
	"runtime"
	"sync"
	"time"
)

// ServerConfig controls the process-local Go runtime profiler endpoint.
type ServerConfig struct {
	Address              string
	BlockProfileRate     int
	MutexProfileFraction int
}

// Server owns an isolated pprof HTTP server and its runtime sampling settings.
type Server struct {
	httpServer *http.Server
	listener   net.Listener
	logger     *slog.Logger
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

// StartServer starts an explicitly configured pprof endpoint.
func StartServer(ctx context.Context, cfg ServerConfig, logger *slog.Logger) (*Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", httppprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)
	server := &Server{
		httpServer: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		listener: listener,
		logger:   logger,
		done:     make(chan struct{}),
	}
	runtime.SetBlockProfileRate(cfg.BlockProfileRate)
	runtime.SetMutexProfileFraction(cfg.MutexProfileFraction)
	logger.Info("profiling.pprof.started", "address", listener.Addr().String())
	go func() {
		if serveErr := server.httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("profiling.pprof.failed", "error", serveErr)
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
		case <-server.done:
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := server.Close(shutdownCtx); closeErr != nil {
			logger.Error("profiling.pprof.shutdown_failed", "error", closeErr)
		}
	}()
	return server, nil
}

// Address returns the actual listener address, including an allocated test port.
func (s *Server) Address() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Close shuts down pprof and disables block and mutex sampling.
func (s *Server) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.closeOnce.Do(func() {
		close(s.done)
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(0)
		s.closeErr = s.httpServer.Shutdown(ctx)
		s.logger.Info("profiling.pprof.stopped")
	})
	return s.closeErr
}
