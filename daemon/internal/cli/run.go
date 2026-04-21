package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/httpapi"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

func newRunCmd() *cobra.Command {
	var bind string
	c := &cobra.Command{
		Use:   "run",
		Short: "Start the WooAgent OS daemon (headless REST API)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			paths, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrDefault(paths.ConfigFile)
			if err != nil {
				return err
			}
			if bind != "" {
				cfg.BindAddr = bind
			}

			st, err := store.Open(ctx, paths.DBFile)
			if err != nil {
				return fmt.Errorf("open store (did you run `wooagent init`?): %w", err)
			}
			defer st.Close()

			am := auth.New(st.DB)
			has, err := am.AnyTokenExists(ctx)
			if err != nil {
				return err
			}
			if !has {
				return fmt.Errorf("no auth tokens present — run `wooagent init` or `wooagent auth token create` first")
			}

			srv := httpapi.New(st, am)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "→ Daemon running on http://%s (headless)\n", cfg.BindAddr)
			fmt.Fprintln(out, "→ Point a UI at this daemon:")
			fmt.Fprintln(out, "    • hosted:  https://ui.wooagent.dev (when available)")
			fmt.Fprintln(out, "    • local:   run `wooagent ui` in another terminal")
			fmt.Fprintln(out, "→ Press Ctrl+C to stop.")

			return httpapi.Run(ctx, cfg.BindAddr, srv.Handler())
		},
	}
	c.Flags().StringVar(&bind, "bind", "", "address to bind (default: value of config.bind_addr or localhost:7777)")
	return c
}
