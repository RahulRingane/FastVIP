package logutil

import (
	"github.com/RahulRingane/FastVIP/pkg/config"
	"go.uber.org/zap"
)

// ServiceFields returns common zap fields for a service configuration.
// These fields provide consistent structured logging across all modules.
func ServiceFields(svc config.ServiceConfig) []zap.Field {
	fields := []zap.Field{
		zap.String("service", svc.Name),
		zap.String("listen", svc.Listen),
		zap.String("protocol", svc.Protocol),
		zap.String("scheduler", svc.Scheduler),
	}
	if svc.FullNAT {
		fields = append(fields, zap.Bool("full_nat", svc.FullNAT))
		if svc.SnatIP != "" {
			fields = append(fields, zap.String("snat_ip", svc.SnatIP))
		}
	}
	return fields
}

// BackendFields returns common zap fields for a backend within a service.
// It includes all service fields plus the backend address.
func BackendFields(svc config.ServiceConfig, backend config.BackendConfig) []zap.Field {
	fields := ServiceFields(svc)
	fields = append(fields, zap.String("backend", backend.Address))
	return fields
}
