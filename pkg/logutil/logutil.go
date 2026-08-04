package logutil

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Loggers holds the logger instances used throughout the application.
type Loggers struct {
	System  *zap.Logger
	Traffic *zap.Logger
}

// SyncAll calls Sync() on all loggers to flush any buffered log entries.
func (l *Loggers) SyncAll() {
	if l.System != nil {
		_ = l.System.Sync()
	}
	if l.Traffic != nil {
		_ = l.Traffic.Sync()
	}
}

// BuildLoggers creates system and traffic loggers based on LogConfig.
//
// System logger outputs to stdout/stderr + ${home}/fastVIP.log.
// Traffic logger outputs to ${home}/traffic.log.
//
// On file creation failure, logs a warning to stderr and falls back to stdout/stderr only.
func BuildLoggers(cfg config.LogConfig) (*Loggers, error) {
	level, err := parseZapLevel(cfg.GetLevel())
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.GetLevel(), err)
	}

	home := cfg.GetHome()
	dirErr := os.MkdirAll(home, 0755)

	// Encoder configs
	consoleEncoderCfg := zap.NewProductionEncoderConfig()
	consoleEncoderCfg.TimeKey = "time"
	consoleEncoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderCfg)

	jsonEncoderCfg := zap.NewProductionEncoderConfig()
	jsonEncoderCfg.TimeKey = "time"
	jsonEncoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	jsonEncoder := zapcore.NewJSONEncoder(jsonEncoderCfg)

	// Build system logger: stdout + file
	systemCores := []zapcore.Core{
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level),
	}
	if dirErr == nil {
		systemFileWriter := newLumberjackWriter(filepath.Join(home, "fastVIP.log"), cfg)
		systemCores = append(systemCores, zapcore.NewCore(jsonEncoder, zapcore.AddSync(systemFileWriter), level))
	} else {
		fmt.Fprintf(os.Stderr, "WARNING: failed to create log directory %q: %v, system log will only output to stdout\n", home, dirErr)
	}
	systemLogger := zap.New(zapcore.NewTee(systemCores...))

	// Build traffic logger: file only (fallback to stdout on error)
	// Traffic log call sites use Debug(), but whether they are written is still
	// controlled by the global log level. This keeps traffic.log gated by
	// global.log.level while still classifying the entries themselves as debug.
	var trafficLogger *zap.Logger
	if dirErr == nil {
		trafficFileWriter := newLumberjackWriter(filepath.Join(home, "traffic.log"), cfg)
		trafficLogger = zap.New(zapcore.NewCore(jsonEncoder, zapcore.AddSync(trafficFileWriter), level))
	} else {
		fmt.Fprintf(os.Stderr, "WARNING: failed to create log directory %q: %v, traffic log will fallback to stdout\n", home, dirErr)
		trafficLogger = zap.New(zapcore.NewCore(jsonEncoder, zapcore.AddSync(os.Stdout), level))
	}

	return &Loggers{
		System:  systemLogger,
		Traffic: trafficLogger,
	}, nil
}

// NewBootstrapLogger creates a minimal stdout-only logger for use before config is loaded.
// It uses info level and console encoding.
func NewBootstrapLogger() *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)
	return zap.New(core)
}

// newLumberjackWriter creates a lumberjack rolling file writer with the given config.
func newLumberjackWriter(filename string, cfg config.LogConfig) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    cfg.GetMaxSize(),
		MaxBackups: cfg.GetMaxBackups(),
		MaxAge:     cfg.GetMaxAge(),
		Compress:   cfg.Compress,
	}
}

// parseZapLevel converts a string log level to a zapcore.Level.
func parseZapLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level: %s", level)
	}
}
