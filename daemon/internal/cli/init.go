package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

// defaultPersonas is the Phase 1 fleet. Seeded into the agents table on init so
// issues can FK to a persona row. Real persona management (deploy/disable)
// lands in Phase 2.
var defaultPersonas = []struct {
	Persona, Name, ModelPreference string
}{
	{"marketing", "Marketing & SEO", "anthropic/claude-sonnet-4-6"},
	{"pricing", "Pricing", "anthropic/claude-sonnet-4-6"},
	{"sales_support", "Sales Support", "anthropic/claude-sonnet-4-6"},
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize local state: ~/.wooagent, SQLite store, initial auth token",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			out := cmd.OutOrStdout()

			paths, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			if err := paths.EnsureDirs(); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ State dir ready at %s\n", paths.Root)

			if _, err := os.Stat(paths.ConfigFile); errors.Is(err, os.ErrNotExist) {
				if err := config.Save(paths.ConfigFile, config.Default()); err != nil {
					return err
				}
				fmt.Fprintf(out, "✓ Wrote default config to %s\n", paths.ConfigFile)
			} else if err != nil {
				return err
			} else {
				fmt.Fprintf(out, "• Config already exists at %s (left untouched)\n", paths.ConfigFile)
			}

			st, err := store.Open(ctx, paths.DBFile)
			if err != nil {
				return err
			}
			defer st.Close()
			fmt.Fprintf(out, "✓ SQLite store initialized at %s (migrations applied)\n", paths.DBFile)

			seeded := 0
			now := time.Now().UTC().Format(time.RFC3339)
			for _, p := range defaultPersonas {
				res, err := st.DB.ExecContext(ctx,
					`INSERT OR IGNORE INTO agents(persona, name, model_preference, enabled, created_at, updated_at) VALUES(?, ?, ?, 1, ?, ?)`,
					p.Persona, p.Name, p.ModelPreference, now, now,
				)
				if err != nil {
					return fmt.Errorf("seed persona %s: %w", p.Persona, err)
				}
				n, _ := res.RowsAffected()
				seeded += int(n)
			}
			if seeded > 0 {
				fmt.Fprintf(out, "✓ Seeded %d default persona(s): marketing, pricing, sales_support\n", seeded)
			} else {
				fmt.Fprintln(out, "• Personas already seeded (left untouched)")
			}

			am := auth.New(st.DB)
			has, err := am.AnyTokenExists(ctx)
			if err != nil {
				return err
			}
			if !has {
				token, err := am.Mint(ctx, "initial")
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "✓ Minted initial auth token (store this — it won't be shown again):\n\n    %s\n\n", token)
			} else {
				fmt.Fprintln(out, "• Auth tokens already present; not minting another. Run `wooagent auth token create` to add one.")
			}

			fmt.Fprintln(out, "\nNext: run `wooagent run` to start the daemon.")
			return nil
		},
	}
}
