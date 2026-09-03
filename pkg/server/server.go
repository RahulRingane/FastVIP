package server

import (
	"context"
	"fmt"
	"time"

	"github.com/RahulRingane/FastVIP/pkg/admin"
	"github.com/RahulRingane/FastVIP/pkg/config"
	"github.com/RahulRingane/FastVIP/pkg/healthcheck"
	httpmanager "github.com/RahulRingane/FastVIP/pkg/http"
	"github.com/RahulRingane/FastVIP/pkg/lvs"
	"github.com/RahulRingane/FastVIP/pkg/metrics"
	"github.com/RahulRingane/FastVIP/pkg/snat"
	"github.com/RahulRingane/FastVIP/pkg/trafficlog"
	"go.uber.org/zap"
)

// Server coordinates all modules and manages the overall service lifecycle.
type Server struct {
	configMgr     *config.Manager
	lvsMgr        *lvs.Manager
	reconciler    *lvs.Reconciler
	healthMgr     *healthcheck.Manager
	snatMgr       snat.Manager
	httpMgr       *httpmanager.Manager
	adminServer   *admin.Server
	logger        *zap.Logger
	trafficLogger *zap.Logger
	collector     *trafficlog.Collector
}

// NewServer initializes all modules and returns a ready-to-run Server.
func NewServer(configPath string, logger *zap.Logger, trafficLogger *zap.Logger) (*Server, error) {
	// Initialize IPVS manager
	lvsMgr, err := lvs.NewManager(logger.Named("lvs"))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize IPVS manager: %w", err)
	}

	return newServerWithManager(configPath, lvsMgr, logger, trafficLogger)
}

// newServerWithManager initializes a Server with a pre-created LVS Manager.
// This allows tests to inject a platform-appropriate Manager instance.
func newServerWithManager(configPath string, lvsMgr *lvs.Manager, logger *zap.Logger, trafficLogger *zap.Logger) (*Server, error) {
	// Initialize config manager
	configMgr, err := config.NewManager(configPath, logger.Named("config"))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config manager: %w", err)
	}

	// Initialize SNAT manager
	snatMgr, err := snat.NewManager(logger.Named("snat"))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SNAT manager: %w", err)
	}

	// Initialize HTTP manager
	httpMgr := httpmanager.NewManager()

	// Publish which backends were linked at build time so a no-op fake build is
	// visible in monitoring, not just in a startup log line.
	metrics.SetFakeBackendActive("ipvs", lvs.BackendKind == lvs.BackendKindFake)
	metrics.SetFakeBackendActive("snat", snat.BackendKind == snat.BackendKindFake)

	server := &Server{
		configMgr:     configMgr,
		lvsMgr:        lvsMgr,
		snatMgr:       snatMgr,
		httpMgr:       httpMgr,
		logger:        logger,
		trafficLogger: trafficLogger,
	}

	// Initialize health check manager with onChange callback that triggers reconcile
	server.healthMgr = healthcheck.NewManager(func() {
		server.triggerReconcile()
		server.updateHealthMetrics()
	}, logger.Named("healthcheck"))

	// Initialize reconciler with health checker and SNAT manager
	server.reconciler = lvs.NewReconciler(lvsMgr, server.healthMgr, snatMgr, logger.Named("reconciler"))

	return server, nil
}

// Run starts the server in daemon mode: performs initial reconcile, starts health checks
// and config watching, then enters the main event loop until context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	cfg := s.configMgr.GetConfig()
	s.logKernelParamPreflight()

	if err := s.syncHTTPServices(cfg); err != nil {
		return fmt.Errorf("failed to start HTTP services: %w", err)
	}

	// Initialize admin server if configured
	if cfg.Global.AdminAddress != "" {
		s.initAdminServer(cfg)
	}

	// Set up config reload callback for metrics
	s.configMgr.SetOnReloadCallback(func() {
		metrics.IncConfigReload()
	})

	// Register health check targets and start checking
	s.healthMgr.UpdateTargets(ctx, cfg.Services)

	// Perform initial reconcile
	if err := s.reconciler.Reconcile(cfg.Services); err != nil {
		s.logger.Error("initial reconcile failed", zap.Error(err))
	} else {
		s.updateHealthMetrics()
	}

	s.syncTrafficCollector(cfg)

	// Start config file watching
	s.configMgr.WatchConfig()
	s.logger.Info("config watcher started")

	// Main event loop
	s.logger.Info("server started, entering main loop")
	for {
		select {
		case <-s.configMgr.OnChange():
			s.logger.Info("config change detected, triggering reconcile")
			newCfg := s.configMgr.GetConfig()
			s.healthMgr.UpdateTargets(ctx, newCfg.Services)
			if err := s.reconciler.Reconcile(newCfg.Services); err != nil {
				s.logger.Error("reconcile after config change failed", zap.Error(err))
			} else {
				s.updateHealthMetrics()
			}
			s.syncTrafficCollector(newCfg)

		case <-ctx.Done():
			s.logger.Info("shutdown signal received, stopping server")
			s.shutdown()
			return nil
		}
	}

}

// RunOnce performs a single reconcile pass and then exits.
// IPVS rules and iptables rules are intentionally preserved after exit —
// cleanup_on_exit does not apply to once mode, whose purpose is to apply
// the desired state and leave it in place.
func (s *Server) RunOnce() error {
	cfg := s.configMgr.GetConfig()
	s.logKernelParamPreflight()

	err := s.reconciler.Reconcile(cfg.Services)
	s.lvsMgr.Close()

	if err != nil {
		return fmt.Errorf("reconcile failed: %w", err)
	}
	return nil
}

// triggerReconcile is called by the health check manager when a backend's health status changes.
func (s *Server) triggerReconcile() {
	cfg := s.configMgr.GetConfig()
	if err := s.reconciler.Reconcile(cfg.Services); err != nil {
		s.logger.Error("reconcile after health change failed", zap.Error(err))
	}

	// Publish the new health status even if the reconcile above failed: the
	// gauge reports the health checker's verdict, which is true regardless of
	// whether IPVS could be reprogrammed to match it. This is the transition
	// that makes the gauge move from 1 to 0 when a backend dies.
	s.updateHealthMetrics()
}

// updateHealthMetrics updates the health status metrics for all configured
// backends. It iterates cfg.Services (the configured backends) rather than
// healthMgr.GetAllStatuses() (only the backends currently being tracked), so
// a backend that is configured but unhealthy still gets an explicit 0 series
// instead of silently having no series at all. Health status is keyed by
// backend address alone, globally across services (see CLAUDE.md), so two
// services sharing a backend address will report the same value here.
func (s *Server) updateHealthMetrics() {
	cfg := s.configMgr.GetConfig()

	for _, svc := range cfg.Services {
		for _, backend := range svc.Backends {
			metrics.SetBackendHealth(svc.Name, backend.Address, s.healthMgr.IsHealthy(backend.Address))
		}
	}
}

func (s *Server) syncTrafficCollector(cfg *config.Config) {
	if cfg == nil {
		return
	}

	if s.collector == nil {
		if !cfg.Global.Log.Traffic.IsEnabled() {
			return
		}

		lvsStats := trafficlog.NewLVSStatsAdapter(s.lvsMgr)

		s.collector = trafficlog.NewCollector(
			lvsStats,
			s.trafficLogger,
			s.logger,
			cfg.Services,
			cfg.Global.Log.Traffic,
		)
		s.collector.Start()
		s.logger.Info("traffic collector started",
			zap.Duration("interval", cfg.Global.Log.Traffic.GetInterval()),
		)
		return
	}

	s.collector.UpdateConfig(cfg.Services, cfg.Global.Log.Traffic)
}

// initAdminServer initializes and starts the admin HTTP server.
func (s *Server) initAdminServer(cfg *config.Config) {
	adminCfg := admin.Config{
		ListenAddr:     cfg.Global.AdminAddress,
		MetricsEnabled: cfg.Global.IsMetricsEnabled(),
		MetricsPath:    cfg.Global.GetMetricsPath(),
	}

	s.adminServer = admin.NewServer(adminCfg, s.logger.Named("admin"))

	// Set up health check function for admin server
	s.adminServer.SetHealthCheckFunc(func() map[string]bool {
		return s.healthMgr.GetAllStatuses()
	})

	// POST /reload re-reads the config file and signals the main loop, which
	// performs the reconcile — the same path the fsnotify watcher takes.
	s.adminServer.SetReloadFunc(s.configMgr.Reload)

	if err := s.adminServer.Start(); err != nil {
		s.logger.Error("failed to start admin server", zap.Error(err))
	}
}

// shutdown gracefully stops all modules.
func (s *Server) shutdown() {
	// Stop admin server first
	if s.adminServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.adminServer.Stop(ctx); err != nil {
			s.logger.Error("failed to stop admin server", zap.Error(err))
		}
	}

	// Stop traffic collector
	if s.collector != nil {
		s.collector.Stop()
		s.logger.Info("traffic collector stopped")
	}

	s.healthMgr.Stop()
	cfg := s.configMgr.GetConfig()
	if cfg.Global.IsCleanupOnExit() {
		if err := s.reconciler.Cleanup(); err != nil {
			s.logger.Error("failed to cleanup IPVS rules", zap.Error(err))
		}
		if err := s.snatMgr.Cleanup(); err != nil {
			s.logger.Error("failed to cleanup SNAT rules", zap.Error(err))
		}
	} else {
		s.logger.Info("cleanup_on_exit is false, preserving IPVS and iptables rules")
	}
	s.lvsMgr.Close()
	s.logger.Info("server stopped")
}

func (s *Server) syncHTTPServices(cfg *config.Config) error {
	for _, svc := range cfg.Services {
		if svc.Mode != "http" {
			continue
		}

		service := &httpmanager.Service{
			Name:     svc.Name,
			Listen:   svc.Listen,
			Backends: make([]string, 0, len(svc.Backends)),
		}

		for _, backend := range svc.Backends {
			service.Backends = append(
				service.Backends,
				backend.Address,
			)
		}

		if err := s.httpMgr.AddService(service); err != nil {
			return err
		}

		if err := s.httpMgr.StartService(service.Name); err != nil {
			return err
		}
	}

	return nil
}
