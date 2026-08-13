package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/RahulRingane/FastVIP/pkg/config"
	"github.com/RahulRingane/FastVIP/pkg/logutil"
	"github.com/RahulRingane/FastVIP/pkg/lvs"
	"github.com/RahulRingane/FastVIP/pkg/server"
	"github.com/RahulRingane/FastVIP/pkg/snat"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	BuildTime   string
	BuildCommit string
	Version     = "0.5.1"
	configPath  string
	showVersion bool
)

func main() {
	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "fastVIP",
		Short: "fastVIP - IPVS based TCP load balancer",
		Long:  "A lightweight four-layer TCP load balancer using Linux IPVS with declarative reconcile mode.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Printf("Version: %s\nBuild commit: %s\nBuild time: %s\nGo version: %s\nIPVS backend: %s\nSNAT backend: %s\n",
					Version,
					BuildCommit,
					BuildTime,
					runtime.Version(),
					lvs.BackendKind,
					snat.BackendKind,
				)
				if lvs.BackendKind == lvs.BackendKindFake || snat.BackendKind == snat.BackendKindFake {
					fmt.Println("\nWARNING: this binary links fake backends and cannot balance traffic.")
					fmt.Println("Rebuild with `make build BUILD_TAGS=integration`.")
				}
				return nil
			}
			return cmd.Help()
		},
	}

	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information")
	rootCmd.AddCommand(newOnceCommand())
	rootCmd.AddCommand(newStartCommand())

	return rootCmd
}

func newOnceCommand() *cobra.Command {
	onceCmd := &cobra.Command{
		Use:   "once",
		Short: "Run a single reconcile pass and exit",
		RunE:  runOnce,
	}

	onceCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to config file")
	return onceCmd
}

func newStartCommand() *cobra.Command {
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the server in daemon mode with signal handling.",
		RunE:  startDaemon,
	}

	startCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to config file")
	return startCmd
}

// startDaemon starts the server in daemon mode with signal handling.
func startDaemon(cmd *cobra.Command, args []string) error {
	// Phase 1: Bootstrap logger (stdout only, info level) for early startup messages
	bootstrapLogger := logutil.NewBootstrapLogger()

	bootstrapLogger.Info("starting fastVIP",
		zap.String("version", Version),
		zap.String("config", configPath),
	)

	// Phase 2: Pre-read log config to build proper loggers before full config load
	logCfg, err := loadLogConfig(configPath)
	if err != nil {
		bootstrapLogger.Warn("failed to pre-read log config, using defaults", zap.Error(err))
		logCfg = config.LogConfig{} // use defaults
	}
	_ = bootstrapLogger.Sync()

	// Phase 3: Build production loggers with file output and rotation
	loggers, err := logutil.BuildLoggers(logCfg)
	if err != nil {
		bootstrapLogger.Fatal("failed to build loggers", zap.Error(err))
	}
	defer loggers.SyncAll()

	logger := loggers.System

	logger.Info("loggers initialized",
		zap.String("level", logCfg.GetLevel()),
		zap.String("home", logCfg.GetHome()),
	)

	// Phase 4: Create server
	srv, err := server.NewServer(configPath, logger, loggers.Traffic)
	if err != nil {
		logger.Fatal("failed to create server", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalChan
		logger.Info("received signal", zap.String("signal", sig.String()))
		cancel()
	}()

	return srv.Run(ctx)
}

// runOnce performs a single reconcile pass and exits.
func runOnce(cmd *cobra.Command, args []string) error {
	// Phase 1: Bootstrap logger
	bootstrapLogger := logutil.NewBootstrapLogger()

	bootstrapLogger.Info("running single reconcile",
		zap.String("version", Version),
		zap.String("config", configPath),
	)

	// Phase 2: Pre-read log config
	logCfg, err := loadLogConfig(configPath)
	if err != nil {
		bootstrapLogger.Warn("failed to pre-read log config, using defaults", zap.Error(err))
		logCfg = config.LogConfig{}
	}
	_ = bootstrapLogger.Sync()

	// Phase 3: Build production loggers
	loggers, err := logutil.BuildLoggers(logCfg)
	if err != nil {
		return fmt.Errorf("failed to build loggers: %w", err)
	}
	defer loggers.SyncAll()

	// Phase 4: Create server
	srv, err := server.NewServer(configPath, loggers.System, loggers.Traffic)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	return srv.RunOnce()
}

// loadLogConfig pre-reads only the global.log section from the config file.
// This allows building proper loggers before the full config validation runs.
func loadLogConfig(path string) (config.LogConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Set defaults matching config.NewManager
	v.SetDefault("global.log.level", "info")
	v.SetDefault("global.log.home", "./logs")
	v.SetDefault("global.log.max_size", 50)
	v.SetDefault("global.log.max_backups", 3)
	v.SetDefault("global.log.max_age", 0)
	v.SetDefault("global.log.compress", false)

	if err := v.ReadInConfig(); err != nil {
		return config.LogConfig{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg struct {
		Global struct {
			Log config.LogConfig `mapstructure:"log"`
		} `mapstructure:"global"`
	}
	if err := v.Unmarshal(&cfg); err != nil {
		return config.LogConfig{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg.Global.Log, nil
}
