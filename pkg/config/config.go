package config

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config represents the top-level configuration structure.
type Config struct {
	Services []ServiceConfig `yaml:"services" mapstructure:"services"`
	Global   GlobalConfig    `yaml:"global"   mapstructure:"global"`
}

// GlobalConfig holds global settings.
type GlobalConfig struct {
	CleanupOnExit  *bool     `yaml:"cleanup_on_exit" mapstructure:"cleanup_on_exit"`
	MetricsEnabled *bool     `yaml:"metrics_enabled" mapstructure:"metrics_enabled"`
	AdminAddress   string    `yaml:"admin_address"   mapstructure:"admin_address"`
	MetricsPath    string    `yaml:"metrics_path"    mapstructure:"metrics_path"`
	Log            LogConfig `yaml:"log"            mapstructure:"log"`
}

// LogConfig holds unified logging configuration.
type LogConfig struct {
	Traffic    TrafficLogConfig `yaml:"traffic"     mapstructure:"traffic"`
	Level      string           `yaml:"level"       mapstructure:"level"`
	Home       string           `yaml:"home"        mapstructure:"home"`
	MaxSize    int              `yaml:"max_size"    mapstructure:"max_size"`
	MaxBackups int              `yaml:"max_backups" mapstructure:"max_backups"`
	MaxAge     int              `yaml:"max_age"     mapstructure:"max_age"`
	Compress   bool             `yaml:"compress"    mapstructure:"compress"`
}

// validLogLevels is the set of supported log levels.
var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// GetLevel returns the log level. Defaults to "info" if not set.
func (l LogConfig) GetLevel() string {
	if l.Level == "" {
		return "info"
	}
	return l.Level
}

// GetHome returns the log directory. Defaults to "./logs" if not set.
func (l LogConfig) GetHome() string {
	if l.Home == "" {
		return "./logs"
	}
	return l.Home
}

// GetMaxSize returns the max size in MB per log file. Defaults to 50.
func (l LogConfig) GetMaxSize() int {
	if l.MaxSize <= 0 {
		return 50
	}
	return l.MaxSize
}

// GetMaxBackups returns the max number of old log files to retain. Defaults to 3.
func (l LogConfig) GetMaxBackups() int {
	if l.MaxBackups <= 0 {
		return 3
	}
	return l.MaxBackups
}

// GetMaxAge returns the max age in days to retain old log files. Defaults to 0 (no limit).
func (l LogConfig) GetMaxAge() int {
	return l.MaxAge
}

// TrafficLogConfig holds traffic logging specific configuration.
type TrafficLogConfig struct {
	Enabled  *bool  `yaml:"enabled"  mapstructure:"enabled"`
	Interval string `yaml:"interval" mapstructure:"interval"`
}

// IsEnabled returns whether traffic logging is enabled. Defaults to true.
func (t TrafficLogConfig) IsEnabled() bool {
	if t.Enabled == nil {
		return true
	}
	return *t.Enabled
}

// GetInterval parses and returns the traffic logging interval.
// Defaults to 15s. Minimum is 5s; values below 5s are clamped to 5s.
func (t TrafficLogConfig) GetInterval() time.Duration {
	if t.Interval == "" {
		return 15 * time.Second
	}
	duration, err := time.ParseDuration(t.Interval)
	if err != nil {
		return 15 * time.Second
	}
	if duration < 5*time.Second {
		return 5 * time.Second
	}
	return duration
}

// IsCleanupOnExit returns whether to clean up IPVS and iptables rules on exit.
// Defaults to true if not explicitly set.
func (g GlobalConfig) IsCleanupOnExit() bool {
	if g.CleanupOnExit == nil {
		return true
	}
	return *g.CleanupOnExit
}

// IsMetricsEnabled returns whether metrics are enabled.
// Defaults to true if not explicitly set.
func (g GlobalConfig) IsMetricsEnabled() bool {
	if g.MetricsEnabled == nil {
		return true
	}
	return *g.MetricsEnabled
}

// GetMetricsPath returns the metrics endpoint path.
// Defaults to "/metrics" if not set.
func (g GlobalConfig) GetMetricsPath() string {
	if g.MetricsPath == "" {
		return "/metrics"
	}
	return g.MetricsPath
}

// ServiceConfig defines a virtual service with its backends and health check settings.
type ServiceConfig struct {
	TrafficLog  *bool             `yaml:"traffic_log"       mapstructure:"traffic_log"`
	Name        string            `yaml:"name"              mapstructure:"name"`
	Listen      string            `yaml:"listen"            mapstructure:"listen"`
	Protocol    string            `yaml:"protocol"          mapstructure:"protocol"`
	Scheduler   string            `yaml:"scheduler"         mapstructure:"scheduler"`
	SnatIP      string            `yaml:"snat_ip"           mapstructure:"snat_ip"`
	Backends    []BackendConfig   `yaml:"backends"          mapstructure:"backends"`
	HealthCheck HealthCheckConfig `yaml:"health_check"      mapstructure:"health_check"`
	FullNAT     bool              `yaml:"full_nat"          mapstructure:"full_nat"`
}

// HealthCheckConfig defines per-service health check parameters.
type HealthCheckConfig struct {
	Enabled            *bool  `yaml:"enabled"              mapstructure:"enabled"`
	Type               string `yaml:"type"                 mapstructure:"type"`
	Interval           string `yaml:"interval"             mapstructure:"interval"`
	Timeout            string `yaml:"timeout"              mapstructure:"timeout"`
	HTTPPath           string `yaml:"http_path"            mapstructure:"http_path"`
	FailCount          int    `yaml:"fail_count"           mapstructure:"fail_count"`
	RiseCount          int    `yaml:"rise_count"           mapstructure:"rise_count"`
	HTTPExpectedStatus int    `yaml:"http_expected_status" mapstructure:"http_expected_status"`
}

// IsEnabled returns whether health check is enabled for this service.
// Defaults to true if not explicitly set.
func (h HealthCheckConfig) IsEnabled() bool {
	if h.Enabled == nil {
		return true
	}
	return *h.Enabled
}

// GetInterval parses and returns the health check interval duration.
// Defaults to 5s if not set or invalid.
func (h HealthCheckConfig) GetInterval() time.Duration {
	if h.Interval == "" {
		return 5 * time.Second
	}
	duration, err := time.ParseDuration(h.Interval)
	if err != nil {
		return 5 * time.Second
	}
	return duration
}

// GetTimeout parses and returns the health check timeout duration.
// Defaults to 3s if not set or invalid.
func (h HealthCheckConfig) GetTimeout() time.Duration {
	if h.Timeout == "" {
		return 3 * time.Second
	}
	duration, err := time.ParseDuration(h.Timeout)
	if err != nil {
		return 3 * time.Second
	}
	return duration
}

// GetType returns the health check type.
// Defaults to "tcp" if not set.
func (h HealthCheckConfig) GetType() string {
	if h.Type == "" {
		return "tcp"
	}
	return h.Type
}

// GetHTTPPath returns the HTTP health check request path.
// Defaults to "/" if not set.
func (h HealthCheckConfig) GetHTTPPath() string {
	if h.HTTPPath == "" {
		return "/"
	}
	return h.HTTPPath
}

// GetHTTPExpectedStatus returns the expected HTTP response status code.
// Defaults to 200 if not set.
func (h HealthCheckConfig) GetHTTPExpectedStatus() int {
	if h.HTTPExpectedStatus <= 0 {
		return 200
	}
	return h.HTTPExpectedStatus
}

// GetFailCount returns the consecutive failure threshold.
// Defaults to 3 if not set.
func (h HealthCheckConfig) GetFailCount() int {
	if h.FailCount <= 0 {
		return 3
	}
	return h.FailCount
}

// GetRiseCount returns the consecutive success threshold.
// Defaults to 2 if not set.
func (h HealthCheckConfig) GetRiseCount() int {
	if h.RiseCount <= 0 {
		return 2
	}
	return h.RiseCount
}

// BackendConfig defines a real server (destination).
type BackendConfig struct {
	Address string `yaml:"address" mapstructure:"address"`
	Weight  int    `yaml:"weight"  mapstructure:"weight"`
}

// validSchedulers is the set of supported IPVS scheduling algorithms.
var validSchedulers = map[string]bool{
	"rr":  true,
	"wrr": true,
	"lc":  true,
	"wlc": true,
	"dh":  true,
	"sh":  true,
}

// validProtocols is the set of supported protocols.
var validProtocols = map[string]bool{
	"tcp": true,
	"udp": true,
}

// Manager handles configuration loading, validation, and hot-reload.
type Manager struct {
	viper      *viper.Viper
	current    *Config
	onChange   chan struct{}
	onReload   func()
	logger     *zap.Logger
	configPath string
	mu         sync.RWMutex
}

// NewManager creates a config Manager, loads and validates the initial configuration.
func NewManager(configPath string, logger *zap.Logger) (*Manager, error) {
	viperInstance := viper.New()
	viperInstance.SetConfigFile(configPath)

	// Set defaults
	viperInstance.SetDefault("global.log.level", "info")
	viperInstance.SetDefault("global.log.home", "./logs")
	viperInstance.SetDefault("global.log.max_size", 50)
	viperInstance.SetDefault("global.log.max_backups", 3)
	viperInstance.SetDefault("global.log.max_age", 0)
	viperInstance.SetDefault("global.log.compress", false)
	viperInstance.SetDefault("global.log.traffic.enabled", true)
	viperInstance.SetDefault("global.log.traffic.interval", "15s")
	viperInstance.SetDefault("global.cleanup_on_exit", true)
	viperInstance.SetDefault("global.metrics_enabled", true)
	viperInstance.SetDefault("global.metrics_path", "/metrics")

	manager := &Manager{
		viper:      viperInstance,
		configPath: configPath,
		onChange:   make(chan struct{}, 1),
		logger:     logger,
	}

	cfg, err := manager.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	manager.current = cfg

	return manager, nil
}

// Load reads the config file, unmarshals it, and validates.
func (m *Manager) Load() (*Config, error) {
	if err := m.viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := m.viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate checks the configuration for correctness.
func Validate(cfg *Config) error {
	// Validate log level
	logLevel := cfg.Global.Log.GetLevel()
	if !validLogLevels[logLevel] {
		return fmt.Errorf("global.log.level: unsupported level %q (supported: debug, info, warn, error)", logLevel)
	}

	// Validate traffic logging interval
	if cfg.Global.Log.Traffic.Interval != "" {
		interval, err := time.ParseDuration(cfg.Global.Log.Traffic.Interval)
		if err != nil {
			return fmt.Errorf("global.log.traffic.interval: invalid duration %q: %w", cfg.Global.Log.Traffic.Interval, err)
		}
		if interval < 5*time.Second {
			return fmt.Errorf("global.log.traffic.interval: minimum interval is 5s, got %v", interval)
		}
	}

	if len(cfg.Services) == 0 {
		return fmt.Errorf("at least one service must be defined")
	}

	nameSet := make(map[string]bool)
	listenSet := make(map[string]bool)

	for i, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("service[%d]: name is required", i)
		}
		if nameSet[svc.Name] {
			return fmt.Errorf("service[%d]: duplicate service name %q", i, svc.Name)
		}
		nameSet[svc.Name] = true

		// Validate listen address
		host, port, err := net.SplitHostPort(svc.Listen)
		if err != nil {
			return fmt.Errorf("service %q: invalid listen address %q: %w", svc.Name, svc.Listen, err)
		}
		if net.ParseIP(host) == nil {
			return fmt.Errorf("service %q: invalid listen IP %q", svc.Name, host)
		}
		if port == "" || port == "0" {
			return fmt.Errorf("service %q: listen port must be a positive number", svc.Name)
		}

		// Validate protocol (default to tcp)
		protocol := svc.Protocol
		if protocol == "" {
			cfg.Services[i].Protocol = "tcp"
			protocol = "tcp"
		}
		if !validProtocols[protocol] {
			return fmt.Errorf("service %q: unsupported protocol %q (supported: tcp, udp)", svc.Name, protocol)
		}

		// Deduplicate by listen address + protocol (IPVS allows same IP:Port for different protocols)
		listenKey := svc.Listen + "/" + protocol
		if listenSet[listenKey] {
			return fmt.Errorf("service %q: duplicate listen address %q for protocol %q", svc.Name, svc.Listen, protocol)
		}
		listenSet[listenKey] = true

		// Validate scheduler
		if !validSchedulers[svc.Scheduler] {
			return fmt.Errorf("service %q: unsupported scheduler %q (supported: rr, wrr, lc, wlc, dh, sh)", svc.Name, svc.Scheduler)
		}

		// Validate health check parameters
		if svc.HealthCheck.IsEnabled() {
			if svc.HealthCheck.Interval != "" {
				if _, err := time.ParseDuration(svc.HealthCheck.Interval); err != nil {
					return fmt.Errorf("service %q: invalid health_check.interval %q: %w", svc.Name, svc.HealthCheck.Interval, err)
				}
			}
			if svc.HealthCheck.Timeout != "" {
				if _, err := time.ParseDuration(svc.HealthCheck.Timeout); err != nil {
					return fmt.Errorf("service %q: invalid health_check.timeout %q: %w", svc.Name, svc.HealthCheck.Timeout, err)
				}
			}

			// Validate health check type
			checkType := svc.HealthCheck.GetType()
			if checkType != "tcp" && checkType != "http" {
				return fmt.Errorf("service %q: unsupported health_check.type %q (supported: tcp, http)", svc.Name, checkType)
			}

			// Validate HTTP-specific parameters
			if checkType == "http" {
				if svc.HealthCheck.HTTPPath != "" && svc.HealthCheck.HTTPPath[0] != '/' {
					return fmt.Errorf("service %q: health_check.http_path must start with '/'", svc.Name)
				}
				if svc.HealthCheck.HTTPExpectedStatus != 0 &&
					(svc.HealthCheck.HTTPExpectedStatus < 100 || svc.HealthCheck.HTTPExpectedStatus > 599) {
					return fmt.Errorf("service %q: health_check.http_expected_status must be between 100 and 599", svc.Name)
				}
			}
		}

		// Validate full_nat and snat_ip
		if svc.SnatIP != "" {
			if !svc.FullNAT {
				return fmt.Errorf("service %q: snat_ip requires full_nat to be enabled", svc.Name)
			}
			if net.ParseIP(svc.SnatIP) == nil {
				return fmt.Errorf("service %q: invalid snat_ip %q", svc.Name, svc.SnatIP)
			}
		}

		// Validate backends
		if len(svc.Backends) == 0 {
			return fmt.Errorf("service %q: at least one backend is required", svc.Name)
		}

		backendSet := make(map[string]bool)
		for j, backend := range svc.Backends {
			if backend.Address == "" {
				return fmt.Errorf("service %q: backend[%d]: address is required", svc.Name, j)
			}
			backendHost, backendPort, err := net.SplitHostPort(backend.Address)
			if err != nil {
				return fmt.Errorf("service %q: backend[%d]: invalid address %q: %w", svc.Name, j, backend.Address, err)
			}
			if net.ParseIP(backendHost) == nil {
				return fmt.Errorf("service %q: backend[%d]: invalid IP %q", svc.Name, j, backendHost)
			}
			if backendPort == "" || backendPort == "0" {
				return fmt.Errorf("service %q: backend[%d]: port must be a positive number", svc.Name, j)
			}
			if backendSet[backend.Address] {
				return fmt.Errorf("service %q: backend[%d]: duplicate address %q", svc.Name, j, backend.Address)
			}
			backendSet[backend.Address] = true

			if backend.Weight <= 0 {
				return fmt.Errorf("service %q: backend[%d]: weight must be a positive integer", svc.Name, j)
			}
		}
	}

	return nil
}

// WatchConfig starts watching the config file for changes.
// On change, it reloads and validates; if valid, updates current config and notifies via onChange channel.
func (m *Manager) WatchConfig() {
	m.viper.OnConfigChange(func(event fsnotify.Event) {
		m.logger.Info("config file changed", zap.String("file", event.Name))

		cfg, err := m.Load()
		if err != nil {
			m.logger.Error("failed to reload config, keeping previous config", zap.Error(err))
			return
		}

		m.mu.Lock()
		m.current = cfg
		m.mu.Unlock()

		m.logger.Info("config reloaded successfully")

		// Increment config reload counter via callback if registered
		if m.onReload != nil {
			m.onReload()
		}

		// Non-blocking send to notify listeners
		select {
		case m.onChange <- struct{}{}:
		default:
		}
	})

	m.viper.WatchConfig()
}

// GetConfig returns a snapshot of the current configuration.
func (m *Manager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// OnChange returns a read-only channel that signals when config has changed.
func (m *Manager) OnChange() <-chan struct{} {
	return m.onChange
}

// SetOnReloadCallback sets a callback function to be called when config is reloaded.
func (m *Manager) SetOnReloadCallback(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReload = fn
}
