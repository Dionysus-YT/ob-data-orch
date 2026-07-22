package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/featuregate"
	"ob-data-orch/internal/localmvp"
	"ob-data-orch/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("control plane stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	localMVP := flag.Bool("local-mvp", false, "run the loopback-only local MVP")
	flag.Parse()
	if *showVersion {
		info := buildinfo.Current()
		fmt.Printf("ob-data-orch control-plane %s (%s, %s)\n", info.Version, info.Commit, info.BuildTime)
		return nil
	}

	if err := featuregate.RequireRealExecutionDisabled(os.LookupEnv); err != nil {
		return err
	}
	appConfig, err := config.LoadControlPlane(os.LookupEnv)
	if err != nil {
		return err
	}

	var handler http.Handler = controlplane.NewHandler(buildinfo.Current())
	var database *store.Store
	if *localMVP {
		if appConfig.ListenAddress != "127.0.0.1:8080" {
			return errors.New("local MVP must bind 127.0.0.1:8080")
		}
		if err := os.MkdirAll("var", 0o700); err != nil {
			return err
		}
		database, err = store.Open(context.Background(), "var/local-mvp.db")
		if err != nil {
			return err
		}
		defer database.Close()
		key, keyErr := credential.LoadOrCreateRootKey("var/local-mvp-root-key.json", "local-mvp-root-v1")
		if keyErr != nil {
			return keyErr
		}
		defer credential.Zero(key)
		keyring, keyErr := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": key})
		if keyErr != nil {
			return keyErr
		}
		handler = controlplane.NewHandlerWithDependencies(buildinfo.Current(), localmvp.Dependencies(database, keyring))
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	server := &http.Server{
		Addr:              appConfig.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("control plane listening",
			"address", appConfig.ListenAddress,
			"stage", "G1",
			"realExecutionEnabled", false,
		)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
