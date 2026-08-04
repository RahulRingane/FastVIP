package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// boolPtr is a helper to create a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// validServiceConfig returns a minimal valid ServiceConfig for testing.
func validServiceConfig() ServiceConfig {
	return ServiceConfig{
		Name:      "test-svc",
		Listen:    "10.0.0.1:80",
		Protocol:  "tcp",
		Scheduler: "rr",
		HealthCheck: HealthCheckConfig{
			Enabled: boolPtr(true),
		},
		Backends: []BackendConfig{
			{Address: "192.168.1.1:8080", Weight: 1},
		},
	}
}

// validConfig returns a minimal valid Config for testing.
func validConfig() *Config {
	svc := validServiceConfig()
	return &Config{
		Global: GlobalConfig{
			Log: LogConfig{
				Level: "info",
			},
		},
		Services: []ServiceConfig{svc},
	}
}

// --- Validate function tests ---

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config to pass validation, got: %v", err)
	}
}

func TestValidate_EmptyServices(t *testing.T) {
	cfg := &Config{Services: []ServiceConfig{}}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty services, got nil")
	}
}

func TestValidate_ServiceNameEmpty(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Name = ""
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty service name, got nil")
	}
}

func TestValidate_ServiceNameDuplicate(t *testing.T) {
	svc1 := validServiceConfig()
	svc2 := validServiceConfig()
	svc2.Listen = "10.0.0.2:80"
	cfg := &Config{Services: []ServiceConfig{svc1, svc2}}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate service name, got nil")
	}
}

func TestValidate_ListenAddressInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Listen = "not-an-address"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid listen address, got nil")
	}
}

func TestValidate_ListenIPInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Listen = "abc:80"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid listen IP, got nil")
	}
}

func TestValidate_ListenPortZero(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Listen = "10.0.0.1:0"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for listen port 0, got nil")
	}
}

func TestValidate_ListenAddressDuplicate(t *testing.T) {
	svc1 := validServiceConfig()
	svc2 := validServiceConfig()
	svc2.Name = "test-svc-2"
	// same listen address and protocol as svc1
	cfg := &Config{Services: []ServiceConfig{svc1, svc2}}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate listen address, got nil")
	}
}

func TestValidate_ProtocolEmptyDefaultsTCP(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Protocol = ""
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected no error when protocol is empty (defaults to tcp), got: %v", err)
	}
	if cfg.Services[0].Protocol != "tcp" {
		t.Errorf("expected protocol to be set to 'tcp', got %q", cfg.Services[0].Protocol)
	}
}

func TestValidate_ProtocolUDP(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Protocol = "udp"
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected valid config with udp protocol, got: %v", err)
	}
}

func TestValidate_ProtocolUnsupported(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Protocol = "sctp"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported protocol, got nil")
	}
}

func TestValidate_SameListenDifferentProtocol(t *testing.T) {
	svc1 := validServiceConfig()
	svc1.Protocol = "tcp"
	svc2 := validServiceConfig()
	svc2.Name = "test-svc-udp"
	svc2.Protocol = "udp"
	// Same listen address, different protocol — should be allowed
	cfg := &Config{Services: []ServiceConfig{svc1, svc2}}
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected same listen address with different protocols to be valid, got: %v", err)
	}
}

func TestValidate_SameListenSameProtocolDuplicate(t *testing.T) {
	svc1 := validServiceConfig()
	svc1.Protocol = "udp"
	svc2 := validServiceConfig()
	svc2.Name = "test-svc-2"
	svc2.Protocol = "udp"
	// Same listen address, same protocol — should be rejected
	cfg := &Config{Services: []ServiceConfig{svc1, svc2}}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate listen address with same protocol, got nil")
	}
}

func TestValidate_SchedulerUnsupported(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Scheduler = "random"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported scheduler, got nil")
	}
}

func TestValidate_SchedulerValidValues(t *testing.T) {
	for _, sched := range []string{"rr", "wrr", "lc", "wlc", "dh", "sh"} {
		cfg := validConfig()
		cfg.Services[0].Scheduler = sched
		if err := Validate(cfg); err != nil {
			t.Errorf("expected scheduler %q to be valid, got: %v", sched, err)
		}
	}
}

func TestValidate_HealthCheckIntervalInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Enabled = boolPtr(true)
	cfg.Services[0].HealthCheck.Interval = "abc"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid health_check.interval, got nil")
	}
}

func TestValidate_HealthCheckTimeoutInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Enabled = boolPtr(true)
	cfg.Services[0].HealthCheck.Timeout = "xyz"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid health_check.timeout, got nil")
	}
}

func TestValidate_HealthCheckTypeHTTP(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Type = "http"
	cfg.Services[0].HealthCheck.HTTPPath = "/healthz"
	cfg.Services[0].HealthCheck.HTTPExpectedStatus = 200
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected valid config with http health check, got: %v", err)
	}
}

func TestValidate_HealthCheckTypeInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Type = "grpc"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported health_check.type, got nil")
	}
}

func TestValidate_HealthCheckHTTPPathInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Type = "http"
	cfg.Services[0].HealthCheck.HTTPPath = "no-leading-slash"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for http_path without leading slash, got nil")
	}
}

func TestValidate_HealthCheckHTTPExpectedStatusInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Type = "http"
	cfg.Services[0].HealthCheck.HTTPExpectedStatus = 999
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for http_expected_status out of range, got nil")
	}
}

func TestGetType_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetType() != "tcp" {
		t.Errorf("expected default type 'tcp', got %q", hc.GetType())
	}
}

func TestGetType_HTTP(t *testing.T) {
	hc := HealthCheckConfig{Type: "http"}
	if hc.GetType() != "http" {
		t.Errorf("expected type 'http', got %q", hc.GetType())
	}
}

func TestGetHTTPPath_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetHTTPPath() != "/" {
		t.Errorf("expected default http_path '/', got %q", hc.GetHTTPPath())
	}
}

func TestGetHTTPPath_Custom(t *testing.T) {
	hc := HealthCheckConfig{HTTPPath: "/healthz"}
	if hc.GetHTTPPath() != "/healthz" {
		t.Errorf("expected http_path '/healthz', got %q", hc.GetHTTPPath())
	}
}

func TestGetHTTPExpectedStatus_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetHTTPExpectedStatus() != 200 {
		t.Errorf("expected default http_expected_status 200, got %d", hc.GetHTTPExpectedStatus())
	}
}

func TestGetHTTPExpectedStatus_Custom(t *testing.T) {
	hc := HealthCheckConfig{HTTPExpectedStatus: 204}
	if hc.GetHTTPExpectedStatus() != 204 {
		t.Errorf("expected http_expected_status 204, got %d", hc.GetHTTPExpectedStatus())
	}
}

// --- full_nat and snat_ip validation tests ---

func TestValidate_FullNATWithSnatIP(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].FullNAT = true
	cfg.Services[0].SnatIP = "10.0.0.1"
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected valid config with full_nat and snat_ip, got: %v", err)
	}
}

func TestValidate_FullNATWithoutSnatIP(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].FullNAT = true
	cfg.Services[0].SnatIP = ""
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected valid config with full_nat and no snat_ip (MASQUERADE), got: %v", err)
	}
}

func TestValidate_SnatIPWithoutFullNAT(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].FullNAT = false
	cfg.Services[0].SnatIP = "10.0.0.1"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for snat_ip without full_nat enabled, got nil")
	}
}

func TestValidate_SnatIPInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].FullNAT = true
	cfg.Services[0].SnatIP = "not-an-ip"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid snat_ip, got nil")
	}
}

func TestValidate_SnatIPEmpty(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].SnatIP = ""
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected valid config with empty snat_ip, got: %v", err)
	}
}

func TestValidate_FullNATDefaultFalse(t *testing.T) {
	cfg := validConfig()
	// FullNAT defaults to false (zero value)
	if cfg.Services[0].FullNAT {
		t.Error("expected FullNAT to default to false")
	}
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected valid config with default full_nat, got: %v", err)
	}
}

func TestValidate_HealthCheckDisabledSkipsIntervalValidation(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].HealthCheck.Enabled = boolPtr(false)
	cfg.Services[0].HealthCheck.Interval = "invalid-duration"
	cfg.Services[0].HealthCheck.Timeout = "also-invalid"
	err := Validate(cfg)
	if err != nil {
		t.Fatalf("expected no error when health check is disabled, got: %v", err)
	}
}

func TestValidate_BackendsEmpty(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends = nil
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty backends, got nil")
	}
}

func TestValidate_BackendAddressEmpty(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends[0].Address = ""
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty backend address, got nil")
	}
}

func TestValidate_BackendAddressInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends[0].Address = "not-valid"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid backend address, got nil")
	}
}

func TestValidate_BackendIPInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends[0].Address = "abc:8080"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid backend IP, got nil")
	}
}

func TestValidate_BackendPortZero(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends[0].Address = "192.168.1.1:0"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for backend port 0, got nil")
	}
}

func TestValidate_BackendAddressDuplicate(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends = append(cfg.Services[0].Backends, BackendConfig{
		Address: "192.168.1.1:8080",
		Weight:  2,
	})
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate backend address, got nil")
	}
}

func TestValidate_BackendWeightZero(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends[0].Weight = 0
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for backend weight 0, got nil")
	}
}

func TestValidate_BackendWeightNegative(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].Backends[0].Weight = -1
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for negative backend weight, got nil")
	}
}

// --- HealthCheckConfig method tests ---

func TestHealthCheckConfig_IsEnabled_DefaultTrue(t *testing.T) {
	hc := HealthCheckConfig{}
	if !hc.IsEnabled() {
		t.Error("expected IsEnabled to return true when Enabled is nil")
	}
}

func TestHealthCheckConfig_IsEnabled_ExplicitTrue(t *testing.T) {
	hc := HealthCheckConfig{Enabled: boolPtr(true)}
	if !hc.IsEnabled() {
		t.Error("expected IsEnabled to return true when Enabled is true")
	}
}

func TestHealthCheckConfig_IsEnabled_ExplicitFalse(t *testing.T) {
	hc := HealthCheckConfig{Enabled: boolPtr(false)}
	if hc.IsEnabled() {
		t.Error("expected IsEnabled to return false when Enabled is false")
	}
}

func TestHealthCheckConfig_GetInterval_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetInterval() != 5*time.Second {
		t.Errorf("expected default interval 5s, got %v", hc.GetInterval())
	}
}

func TestHealthCheckConfig_GetInterval_Invalid(t *testing.T) {
	hc := HealthCheckConfig{Interval: "invalid"}
	if hc.GetInterval() != 5*time.Second {
		t.Errorf("expected fallback interval 5s for invalid value, got %v", hc.GetInterval())
	}
}

func TestHealthCheckConfig_GetInterval_Valid(t *testing.T) {
	hc := HealthCheckConfig{Interval: "10s"}
	if hc.GetInterval() != 10*time.Second {
		t.Errorf("expected interval 10s, got %v", hc.GetInterval())
	}
}

func TestHealthCheckConfig_GetTimeout_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetTimeout() != 3*time.Second {
		t.Errorf("expected default timeout 3s, got %v", hc.GetTimeout())
	}
}

func TestHealthCheckConfig_GetTimeout_Invalid(t *testing.T) {
	hc := HealthCheckConfig{Timeout: "bad"}
	if hc.GetTimeout() != 3*time.Second {
		t.Errorf("expected fallback timeout 3s for invalid value, got %v", hc.GetTimeout())
	}
}

func TestHealthCheckConfig_GetTimeout_Valid(t *testing.T) {
	hc := HealthCheckConfig{Timeout: "7s"}
	if hc.GetTimeout() != 7*time.Second {
		t.Errorf("expected timeout 7s, got %v", hc.GetTimeout())
	}
}

func TestHealthCheckConfig_GetFailCount_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetFailCount() != 3 {
		t.Errorf("expected default fail_count 3, got %d", hc.GetFailCount())
	}
}

func TestHealthCheckConfig_GetFailCount_Negative(t *testing.T) {
	hc := HealthCheckConfig{FailCount: -1}
	if hc.GetFailCount() != 3 {
		t.Errorf("expected default fail_count 3 for negative value, got %d", hc.GetFailCount())
	}
}

func TestHealthCheckConfig_GetFailCount_Valid(t *testing.T) {
	hc := HealthCheckConfig{FailCount: 5}
	if hc.GetFailCount() != 5 {
		t.Errorf("expected fail_count 5, got %d", hc.GetFailCount())
	}
}

func TestHealthCheckConfig_GetRiseCount_Default(t *testing.T) {
	hc := HealthCheckConfig{}
	if hc.GetRiseCount() != 2 {
		t.Errorf("expected default rise_count 2, got %d", hc.GetRiseCount())
	}
}

func TestHealthCheckConfig_GetRiseCount_Negative(t *testing.T) {
	hc := HealthCheckConfig{RiseCount: -1}
	if hc.GetRiseCount() != 2 {
		t.Errorf("expected default rise_count 2 for negative value, got %d", hc.GetRiseCount())
	}
}

func TestHealthCheckConfig_GetRiseCount_Valid(t *testing.T) {
	hc := HealthCheckConfig{RiseCount: 4}
	if hc.GetRiseCount() != 4 {
		t.Errorf("expected rise_count 4, got %d", hc.GetRiseCount())
	}
}

// --- Manager loading tests ---

const validYAML = `
global:
  log:
    level: info
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: wrr
    health_check:
      enabled: true
      interval: 5s
      timeout: 3s
      fail_count: 3
      rise_count: 2
    backends:
      - address: 192.168.1.10:8080
        weight: 5
      - address: 192.168.1.11:8080
        weight: 3
`

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}
	return path
}

func TestManager_LoadValidYAML(t *testing.T) {
	path := writeTestYAML(t, validYAML)

	mgr, err := NewManager(path, zap.NewNop())
	if err != nil {
		t.Fatalf("expected NewManager to succeed, got: %v", err)
	}

	cfg := mgr.GetConfig()
	if cfg == nil {
		t.Fatal("expected GetConfig to return non-nil config")
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Name != "web-service" {
		t.Errorf("expected service name 'web-service', got %q", cfg.Services[0].Name)
	}
	if cfg.Services[0].Scheduler != "wrr" {
		t.Errorf("expected scheduler 'wrr', got %q", cfg.Services[0].Scheduler)
	}
	if len(cfg.Services[0].Backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(cfg.Services[0].Backends))
	}
}

func TestManager_LoadNonExistentFile(t *testing.T) {
	_, err := NewManager("/nonexistent/path/config.yaml", zap.NewNop())
	if err == nil {
		t.Fatal("expected error for non-existent config file, got nil")
	}
}

func TestManager_LoadInvalidYAML(t *testing.T) {
	path := writeTestYAML(t, `{{{invalid yaml`)
	_, err := NewManager(path, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestManager_LoadValidationFailure(t *testing.T) {
	invalidCfg := `
global:
  log:
    level: info
services:
  - name: bad-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    backends: []
`
	path := writeTestYAML(t, invalidCfg)
	_, err := NewManager(path, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for config that fails validation, got nil")
	}
}

func TestManager_OnChangeChannel(t *testing.T) {
	path := writeTestYAML(t, validYAML)
	mgr, err := NewManager(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	ch := mgr.OnChange()
	if ch == nil {
		t.Fatal("expected OnChange to return non-nil channel")
	}
}

// --- GlobalConfig.IsCleanupOnExit tests ---

func TestGlobalConfig_IsCleanupOnExit_DefaultTrue(t *testing.T) {
	g := GlobalConfig{}
	if !g.IsCleanupOnExit() {
		t.Error("expected IsCleanupOnExit to return true when CleanupOnExit is nil")
	}
}

func TestGlobalConfig_IsCleanupOnExit_ExplicitTrue(t *testing.T) {
	g := GlobalConfig{CleanupOnExit: boolPtr(true)}
	if !g.IsCleanupOnExit() {
		t.Error("expected IsCleanupOnExit to return true when CleanupOnExit is explicitly true")
	}
}

func TestGlobalConfig_IsCleanupOnExit_ExplicitFalse(t *testing.T) {
	g := GlobalConfig{CleanupOnExit: boolPtr(false)}
	if g.IsCleanupOnExit() {
		t.Error("expected IsCleanupOnExit to return false when CleanupOnExit is explicitly false")
	}
}

func TestManager_LoadYAML_CleanupOnExitDefault(t *testing.T) {
	// cleanup_on_exit not set in YAML — should default to true
	path := writeTestYAML(t, validYAML)
	mgr, err := NewManager(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if !cfg.Global.IsCleanupOnExit() {
		t.Error("expected IsCleanupOnExit to return true when not set in config")
	}
}

// --- LogConfig getter tests ---

func TestLogConfig_GetLevel_Default(t *testing.T) {
	lc := LogConfig{}
	if lc.GetLevel() != "info" {
		t.Errorf("expected default level 'info', got %q", lc.GetLevel())
	}
}

func TestLogConfig_GetLevel_Valid(t *testing.T) {
	lc := LogConfig{Level: "debug"}
	if lc.GetLevel() != "debug" {
		t.Errorf("expected level 'debug', got %q", lc.GetLevel())
	}
}

func TestLogConfig_GetHome_Default(t *testing.T) {
	lc := LogConfig{}
	if lc.GetHome() != "./logs" {
		t.Errorf("expected default home './logs', got %q", lc.GetHome())
	}
}

func TestLogConfig_GetHome_Custom(t *testing.T) {
	lc := LogConfig{Home: "/var/log/fastVIP"}
	if lc.GetHome() != "/var/log/fastVIP" {
		t.Errorf("expected home '/var/log/fastVIP', got %q", lc.GetHome())
	}
}

func TestLogConfig_GetMaxSize_Default(t *testing.T) {
	lc := LogConfig{}
	if lc.GetMaxSize() != 50 {
		t.Errorf("expected default max_size 50, got %d", lc.GetMaxSize())
	}
}

func TestLogConfig_GetMaxBackups_Default(t *testing.T) {
	lc := LogConfig{}
	if lc.GetMaxBackups() != 3 {
		t.Errorf("expected default max_backups 3, got %d", lc.GetMaxBackups())
	}
}

func TestLogConfig_GetMaxAge_Default(t *testing.T) {
	lc := LogConfig{}
	if lc.GetMaxAge() != 0 {
		t.Errorf("expected default max_age 0, got %d", lc.GetMaxAge())
	}
}

// --- TrafficLogConfig getter tests ---

func TestTrafficLogConfig_IsEnabled_Default(t *testing.T) {
	tc := TrafficLogConfig{}
	if !tc.IsEnabled() {
		t.Error("expected IsEnabled to return true when Enabled is nil")
	}
}

func TestTrafficLogConfig_IsEnabled_False(t *testing.T) {
	tc := TrafficLogConfig{Enabled: boolPtr(false)}
	if tc.IsEnabled() {
		t.Error("expected IsEnabled to return false when Enabled is false")
	}
}

func TestTrafficLogConfig_GetInterval_Default(t *testing.T) {
	tc := TrafficLogConfig{}
	if tc.GetInterval() != 15*time.Second {
		t.Errorf("expected default interval 15s, got %v", tc.GetInterval())
	}
}

func TestTrafficLogConfig_GetInterval_TooSmall(t *testing.T) {
	tc := TrafficLogConfig{Interval: "2s"}
	if tc.GetInterval() != 5*time.Second {
		t.Errorf("expected clamped interval 5s for too-small value, got %v", tc.GetInterval())
	}
}

func TestTrafficLogConfig_GetInterval_Valid(t *testing.T) {
	tc := TrafficLogConfig{Interval: "30s"}
	if tc.GetInterval() != 30*time.Second {
		t.Errorf("expected interval 30s, got %v", tc.GetInterval())
	}
}

// --- Validate log-related tests ---

func TestValidate_LogLevelInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Global.Log.Level = "trace"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
}

func TestValidate_TrafficLogTrue(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].TrafficLog = boolPtr(true)
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected traffic_log=true to be valid, got: %v", err)
	}
}

func TestValidate_TrafficLogFalse(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].TrafficLog = boolPtr(false)
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected traffic_log=false to be valid, got: %v", err)
	}
}

func TestValidate_TrafficLogNil(t *testing.T) {
	cfg := validConfig()
	cfg.Services[0].TrafficLog = nil
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected traffic_log=nil (default disabled) to be valid, got: %v", err)
	}
}

func TestValidate_TrafficIntervalTooSmall(t *testing.T) {
	cfg := validConfig()
	cfg.Global.Log.Traffic.Interval = "2s"
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for traffic interval < 5s, got nil")
	}
}

func TestValidate_TrafficIntervalValid(t *testing.T) {
	cfg := validConfig()
	cfg.Global.Log.Traffic.Interval = "30s"
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid traffic interval, got: %v", err)
	}
}

func TestManager_LoadYAML_NewLogConfig(t *testing.T) {
	yaml := `
global:
  log:
    level: debug
    home: /tmp/fastVIP-logs
    max_size: 100
    traffic:
      enabled: false
      interval: 30s
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 1
`
	path := writeTestYAML(t, yaml)
	mgr, err := NewManager(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if cfg.Global.Log.GetLevel() != "debug" {
		t.Errorf("expected log level 'debug', got %q", cfg.Global.Log.GetLevel())
	}
	if cfg.Global.Log.GetHome() != "/tmp/fastVIP-logs" {
		t.Errorf("expected log home '/tmp/fastVIP-logs', got %q", cfg.Global.Log.GetHome())
	}
	if cfg.Global.Log.GetMaxSize() != 100 {
		t.Errorf("expected max_size 100, got %d", cfg.Global.Log.GetMaxSize())
	}
	if cfg.Global.Log.Traffic.IsEnabled() {
		t.Error("expected traffic logging to be disabled")
	}
	if cfg.Global.Log.Traffic.GetInterval() != 30*time.Second {
		t.Errorf("expected traffic interval 30s, got %v", cfg.Global.Log.Traffic.GetInterval())
	}
}

func TestManager_LoadYAML_CleanupOnExitFalse(t *testing.T) {
	yaml := `
global:
  log:
    level: info
  cleanup_on_exit: false
services:
  - name: web-service
    listen: 10.0.0.1:80
    protocol: tcp
    scheduler: rr
    health_check:
      enabled: false
    backends:
      - address: 192.168.1.10:8080
        weight: 1
`
	path := writeTestYAML(t, yaml)
	mgr, err := NewManager(path, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	cfg := mgr.GetConfig()
	if cfg.Global.IsCleanupOnExit() {
		t.Error("expected IsCleanupOnExit to return false when cleanup_on_exit: false in config")
	}
}
