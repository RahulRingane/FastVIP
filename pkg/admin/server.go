package admin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server provides an HTTP admin interface for metrics and health checks.
type Server struct {
	listener        net.Listener
	logger          *zap.Logger
	server          *http.Server
	healthCheckFunc func() map[string]bool
	listenAddr      string
	actualAddr      string
	metricsPath     string
	metricsEnabled  bool
}

// Config holds the configuration for the admin server.
type Config struct {
	ListenAddr     string
	MetricsPath    string
	MetricsEnabled bool
}

// NewServer creates a new admin server.
func NewServer(cfg Config, logger *zap.Logger) *Server {
	return &Server{
		listenAddr:     cfg.ListenAddr,
		metricsEnabled: cfg.MetricsEnabled,
		metricsPath:    cfg.MetricsPath,
		logger:         logger,
	}
}

// SetHealthCheckFunc sets the function used to retrieve health status.
func (s *Server) SetHealthCheckFunc(fn func() map[string]bool) {
	s.healthCheckFunc = fn
}

// Start starts the admin HTTP server in a background goroutine.
// Returns an error if the server cannot start.
func (s *Server) Start() error {
	if s.listenAddr == "" {
		s.logger.Info("admin server disabled: no listen address configured")
		return nil
	}

	mux := http.NewServeMux()

	// Register metrics endpoint if enabled
	if s.metricsEnabled {
		metricsPath := s.metricsPath
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		mux.Handle(metricsPath, promhttp.Handler())
		s.logger.Info("metrics endpoint registered", zap.String("path", metricsPath))
	}

	// Register health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Register config reload endpoint (placeholder for future use)
	mux.HandleFunc("/reload", s.handleReload)

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Validate address format
	if _, _, err := net.SplitHostPort(s.listenAddr); err != nil {
		return fmt.Errorf("invalid admin listen address %q: %w", s.listenAddr, err)
	}

	// Create listener to get actual address (important for :0 port)
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener
	s.actualAddr = listener.Addr().String()

	go func() {
		s.logger.Info("admin server starting", zap.String("addr", s.actualAddr))
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("admin server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the admin server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("admin server stopping")
	return s.server.Shutdown(ctx)
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Get backend health status if available
	var backendHealth map[string]bool
	if s.healthCheckFunc != nil {
		backendHealth = s.healthCheckFunc()
	}

	response := fmt.Sprintf(`{"status":"healthy","backends":%s}`, formatHealthJSON(backendHealth))
	w.Write([]byte(response))
}

// handleReload handles config reload requests (placeholder).
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement config reload trigger
	s.logger.Info("config reload requested via admin API")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"reload triggered"}`))
}

// formatHealthJSON converts health map to JSON string.
func formatHealthJSON(health map[string]bool) string {
	if health == nil {
		return "{}"
	}

	var parts []string
	for k, v := range health {
		value := "false"
		if v {
			value = "true"
		}
		parts = append(parts, fmt.Sprintf("%q:%s", k, value))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// IsEnabled returns true if the admin server is configured to run.
func (s *Server) IsEnabled() bool {
	return s.listenAddr != ""
}

// Addr returns the actual address the server is listening on.
// This is useful when using :0 to get an available port.
func (s *Server) Addr() string {
	return s.actualAddr
}
