package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/featuregate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		info := buildinfo.Current()
		fmt.Printf("ob-data-orch agent %s (%s, %s)\n", info.Version, info.Commit, info.BuildTime)
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
