package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
)

func newAuthTokenCreateCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "create",
		Short: "Mint a new auth token for a UI or teammate",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if name == "" {
				name = "manual"
			}

			paths, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			st, err := store.Open(ctx, paths.DBFile)
			if err != nil {
				return err
			}
			defer st.Close()

			token, err := auth.New(st.DB).Mint(ctx, name)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}
	c.Flags().StringVarP(&name, "name", "n", "", "friendly name recorded with the token (default: manual)")
	return c
}
