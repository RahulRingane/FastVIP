package logutil

import (
	"testing"

	"github.com/fastVIP/fastVIP/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestServiceFields(t *testing.T) {
	svc := config.ServiceConfig{
		Name:      "web-service",
		Listen:    "10.0.0.1:80",
		Protocol:  "tcp",
		Scheduler: "wrr",
	}

	fields := ServiceFields(svc)

	// Convert fields to a map for easy assertion
	fieldMap := fieldsToMap(fields)

	assertField(t, fieldMap, "service", "web-service")
	assertField(t, fieldMap, "listen", "10.0.0.1:80")
	assertField(t, fieldMap, "protocol", "tcp")
	assertField(t, fieldMap, "scheduler", "wrr")

	// full_nat should not be present when FullNAT is false
	if _, ok := fieldMap["full_nat"]; ok {
		t.Error("expected full_nat field to be absent when FullNAT is false")
	}
}

func TestBackendFields(t *testing.T) {
	svc := config.ServiceConfig{
		Name:      "api-service",
		Listen:    "10.0.0.2:443",
		Protocol:  "tcp",
		Scheduler: "rr",
	}
	backend := config.BackendConfig{
		Address: "192.168.1.10:8080",
		Weight:  5,
	}

	fields := BackendFields(svc, backend)
	fieldMap := fieldsToMap(fields)

	// Should include service fields
	assertField(t, fieldMap, "service", "api-service")
	assertField(t, fieldMap, "listen", "10.0.0.2:443")

	// Should include backend field
	assertField(t, fieldMap, "backend", "192.168.1.10:8080")
}

func TestServiceFields_FullNAT(t *testing.T) {
	svc := config.ServiceConfig{
		Name:      "dns-service",
		Listen:    "10.0.0.3:53",
		Protocol:  "udp",
		Scheduler: "rr",
		FullNAT:   true,
		SnatIP:    "10.0.0.3",
	}

	fields := ServiceFields(svc)
	fieldMap := fieldsToMap(fields)

	assertField(t, fieldMap, "service", "dns-service")
	assertField(t, fieldMap, "protocol", "udp")

	// full_nat should be present
	if _, ok := fieldMap["full_nat"]; !ok {
		t.Error("expected full_nat field to be present when FullNAT is true")
	}

	// snat_ip should be present
	assertField(t, fieldMap, "snat_ip", "10.0.0.3")
}

func TestServiceFields_FullNATWithoutSnatIP(t *testing.T) {
	svc := config.ServiceConfig{
		Name:      "dns-service",
		Listen:    "10.0.0.3:53",
		Protocol:  "udp",
		Scheduler: "rr",
		FullNAT:   true,
		SnatIP:    "", // MASQUERADE mode
	}

	fields := ServiceFields(svc)
	fieldMap := fieldsToMap(fields)

	// full_nat should be present
	if _, ok := fieldMap["full_nat"]; !ok {
		t.Error("expected full_nat field to be present when FullNAT is true")
	}

	// snat_ip should NOT be present when empty
	if _, ok := fieldMap["snat_ip"]; ok {
		t.Error("expected snat_ip field to be absent when SnatIP is empty")
	}
}

// fieldsToMap converts zap fields to a map for easy assertion.
func fieldsToMap(fields []zap.Field) map[string]string {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}
	result := make(map[string]string)
	for k, v := range enc.Fields {
		switch val := v.(type) {
		case string:
			result[k] = val
		default:
			// For non-string types, just record that the key exists
			result[k] = "__exists__"
		}
	}
	return result
}

// assertField checks that a field exists with the expected string value.
func assertField(t *testing.T, fieldMap map[string]string, key, expected string) {
	t.Helper()
	val, ok := fieldMap[key]
	if !ok {
		t.Errorf("expected field %q to be present", key)
		return
	}
	if val != expected {
		t.Errorf("expected field %q = %q, got %q", key, expected, val)
	}
}
