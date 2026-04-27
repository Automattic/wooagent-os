package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/httpapi"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
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

			// MCP wiring is optional. The approve endpoint requires it; everything
			// else (kanban, issue detail, list/create) runs without. Storing
			// credentials in env vars rather than the DB is intentional for v0.1
			// — secrets storage lands with the keychain integration in Phase 3.
			mcpClient := loadMCPClient(cmd.OutOrStdout())

			srv := httpapi.New(st, am, mcpClient)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "→ Daemon running on http://%s (headless)\n", cfg.BindAddr)
			if mcpClient != nil {
				fmt.Fprintln(out, "→ MCP store wired — Approve will write through to the connected store.")
			} else {
				fmt.Fprintln(out, "→ MCP not configured — Approve will return 503 (set WOOAGENT_MCP_URL/USER/APP_PASSWORD to enable).")
			}
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

// loadMCPClient assembles an MCP client from env vars if all three are set;
// otherwise returns nil and writes a hint to out. The caller decides whether
// nil is fatal (it isn't, for v0.1).
func loadMCPClient(out interface{ Write([]byte) (int, error) }) *mcp.Client {
	url := os.Getenv("WOOAGENT_MCP_URL")
	user := os.Getenv("WOOAGENT_MCP_USER")
	pass := os.Getenv("WOOAGENT_MCP_APP_PASSWORD")
	if url == "" || user == "" || pass == "" {
		return nil
	}
	return mcp.NewClient(mcp.Config{
		Endpoint: url,
		Username: user,
		// WP shows app passwords with spaces for readability; strip them so
		// pasted values from the admin UI work without preprocessing.
		Password: strings.ReplaceAll(pass, " ", ""),
	})
}
