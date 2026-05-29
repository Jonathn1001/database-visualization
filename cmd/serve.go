package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/elgnas/dbviz/internal/obs"
	"github.com/elgnas/dbviz/internal/server"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	// Register database engine adapters.
	_ "github.com/elgnas/dbviz/internal/adapter/postgres"
)

func newServeCmd() *cobra.Command {
	var (
		bind          string
		logLevel      string
		auditOn       bool
		allowInsecure bool
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the dbviz server",
		RunE: func(c *cobra.Command, _ []string) error {
			dev := os.Getenv("DBVIZ_DEV") == "1"
			log := setupLogger(dev, logLevel)
			slog.SetDefault(log)

			// Refuse to expose the unauthenticated API on a non-loopback
			// interface unless the operator explicitly opts in (§22.5).
			if err := guardBindAddr(bind, allowInsecure); err != nil {
				return err
			}

			srv := server.New(server.Config{
				Dev:          dev,
				BindAddr:     bind,
				Version:      buildVersion,
				Logger:       log,
				AuditEnabled: auditOn,
			})

			// In production the binary serves the SPA itself, so open a browser.
			// In dev the user visits the Vite server on :5173, and air restarts
			// would re-open the browser repeatedly — so skip it.
			if !dev {
				go func() {
					time.Sleep(300 * time.Millisecond)
					_ = browser.OpenURL("http://" + bind)
				}()
			}

			errCh := make(chan error, 1)
			go func() { errCh <- srv.Start() }()

			ctx, stop := signal.NotifyContext(c.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				log.Info("shutting down")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return srv.Shutdown(shutdownCtx)
			}
		},
	}

	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1:7777", "address to bind the server")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error (default: debug in dev, info otherwise)")
	cmd.Flags().BoolVar(&auditOn, "audit", true, "record a query audit log to ~/.dbviz/audit (§18)")
	cmd.Flags().BoolVar(&allowInsecure, "i-understand-this-is-insecure", false, "permit binding to a non-loopback address (no auth; exposes DB read access)")
	return cmd
}

// guardBindAddr rejects non-loopback bind addresses unless explicitly allowed.
// The API has no authentication and grants read access to every connected
// database, so a public bind is dangerous without informed consent (§22.5).
func guardBindAddr(bind string, allowInsecure bool) error {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return fmt.Errorf("invalid --bind %q: %w", bind, err)
	}
	if isLoopbackHost(host) || allowInsecure {
		return nil
	}
	return fmt.Errorf("refusing to bind to non-loopback address %q: the API has no authentication and exposes read access to connected databases — pass --i-understand-this-is-insecure to override", bind)
}

func isLoopbackHost(h string) bool {
	switch h {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func setupLogger(dev bool, level string) *slog.Logger {
	lvl := slog.LevelInfo
	if dev {
		lvl = slog.LevelDebug
	}
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	// Dev: human-readable text to stderr. Prod: JSON to stderr.
	// TODO(phase1): prod file output + rotation per DESIGN_PLAN §17.2/§17.4.
	return obs.Setup(lvl, os.Stderr, dev)
}
