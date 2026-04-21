package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/version"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "wooagent",
		Short: "WooAgent OS — local-first agent OS for WooCommerce operators",
		Long: `WooAgent OS is a local-first agent operating system for WooCommerce.
The daemon runs a fleet of purpose-built agent personas against a connected
store via MCP. See wooagent-os-prd V2.md for the full spec.`,
		SilenceUsage: true,
	}

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newRunCmd(),
		newUICmd(),
		newStoreCmd(),
		newAgentCmd(),
		newModelCmd(),
		newIssueCmd(),
		newAuthCmd(),
		newLogsCmd(),
		newAbilitiesCmd(),
		newExportCmd(),
	)

	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print wooagent version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "wooagent %s (schema %s)\n", version.Version, version.SchemaVersion)
			return nil
		},
	}
}
