package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MikeS071/agent-swarm/internal/config"
	"github.com/MikeS071/agent-swarm/internal/dispatcher"
	"github.com/MikeS071/agent-swarm/internal/server"
	"github.com/MikeS071/agent-swarm/internal/tracker"
	"github.com/spf13/cobra"
)

var servePort int
var serveCORS []string
var serveAuthToken string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run HTTP API server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("port") {
			cfg.Serve.Port = servePort
		}
		if cmd.Flags().Changed("cors") {
			cfg.Serve.CORS = serveCORS
		}
		if cmd.Flags().Changed("auth-token") {
			cfg.Serve.AuthToken = serveAuthToken
		}
		if cfg.Serve.Port <= 0 {
			cfg.Serve.Port = 8090
		}

		trackerPath := resolveFromConfig(cfgFile, cfg.Project.Tracker)
		cfg.Project.Tracker = trackerPath
		tr, err := tracker.Load(trackerPath)
		if err != nil {
			return err
		}
		d := dispatcher.New(cfg, tr)

		be, err := buildBackend(cfg)
		if err != nil {
			return err
		}

		watchdog := server.NewMemoryWatchdog(parseWatchInterval(cfg.Watchdog.Interval))
		s := server.New(cfg, tr, d, be, watchdog, log.Default())

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		watchdog.Start(ctx)

		httpSrv := &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Serve.Port),
			Handler: s.Router(),
		}

		errCh := make(chan error, 1)
		go func() {
			if runErr := httpSrv.ListenAndServe(); runErr != nil && !errors.Is(runErr, http.ErrServerClosed) {
				errCh <- runErr
			}
		}()

		fmt.Fprintf(cmd.OutOrStdout(), "swarm serve listening on :%d\n", cfg.Serve.Port)

		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := httpSrv.Shutdown(shutdownCtx); err != nil {
				return err
			}
			return s.Close(shutdownCtx)
		case err := <-errCh:
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.Close(shutdownCtx)
			return err
		}
	},
}


func parseWatchInterval(raw string) time.Duration {
	if raw == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 8090, "HTTP server port")
	serveCmd.Flags().StringSliceVar(&serveCORS, "cors", nil, "Allowed CORS origins (repeatable)")
	serveCmd.Flags().StringVar(&serveAuthToken, "auth-token", "", "Bearer token for API auth")
	rootCmd.AddCommand(serveCmd)
}
