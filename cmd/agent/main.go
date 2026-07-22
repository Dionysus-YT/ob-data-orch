package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/featuregate"
	"ob-data-orch/internal/jdbcprobe"
)

func main() {
	if err := run(); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	checkRuntime := flag.Bool("check-runtime", false, "verify local Java and OBDUMPER runtime")
	flag.Parse()
	if *showVersion {
		info := buildinfo.Current()
		fmt.Printf("ob-data-orch agent %s (%s, %s)\n", info.Version, info.Commit, info.BuildTime)
		return nil
	}
	if *checkRuntime {
		runtimeConfig, err := config.LoadAgentRuntime(os.LookupEnv)
		if err != nil {
			return errors.New("agent runtime configuration is invalid")
		}
		if _, err := jdbcprobe.DiscoverRuntime(runtimeConfig.JavaPath, runtimeConfig.ToolHome, runtimeConfig.Environment); err != nil {
			return errors.New("agent JDBC runtime verification failed")
		}
		fmt.Println("agent runtime verified: Java and JDBC connector are ready")
		return nil
	}

	if err := featuregate.RequireRealExecutionDisabled(os.LookupEnv); err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	logger.Info("agent idle",
		"stage", "G1",
		"networkEnabled", false,
		"realExecutionEnabled", false,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	return nil
}
