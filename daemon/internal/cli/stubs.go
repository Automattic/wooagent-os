package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// errNotImplemented is the standard stub error for CLI surfaces that are
// scheduled in later phases. Keeping the verb bound at phase-start means the
// CLI surface matches the PRD §13.3 from day one; real behaviour slots in
// without restructuring arguments or flags.
var errNotImplemented = errors.New("not implemented yet — see WooAgent_plan.md for phase schedule")

func stub(use, short string, subs ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(subs) == 0 {
				return errNotImplemented
			}
			return cmd.Help()
		},
	}
	c.AddCommand(subs...)
	return c
}

func stubLeaf(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return errNotImplemented
		},
	}
}

func newUICmd() *cobra.Command {
	return stubLeaf("ui", "Serve the bundled standalone UI locally")
}

func newStoreCmd() *cobra.Command {
	return stub("store", "Manage connected WooCommerce stores",
		stubLeaf("add <url>", "Add a store (MCP or REST)"),
		stubLeaf("list", "List connected stores"),
		stubLeaf("pair <url>", "Pair with a store via device-pairing flow"),
		stubLeaf("unpair <url>", "Revoke the local token for a store"),
		stubLeaf("abilities [store]", "List discovered abilities"),
	)
}

func newAgentCmd() *cobra.Command {
	return stub("agent", "Manage agent personas",
		stubLeaf("list", "List configured personas"),
		stubLeaf("deploy <persona>", "Add a new persona from template"),
		stubLeaf("disable <name>", "Disable a persona"),
	)
}

func newModelCmd() *cobra.Command {
	return stub("model", "Manage model providers",
		stub("provider", "Manage model providers",
			stubLeaf("add <name>", "Add a model provider"),
			stubLeaf("test <name>", "Test a configured provider"),
		),
	)
}

func newIssueCmd() *cobra.Command {
	return stub("issue", "Manage issues",
		stubLeaf("create", "Create a new issue"),
		stubLeaf("list", "List issues"),
	)
}

func newAuthCmd() *cobra.Command {
	return stub("auth", "Manage local auth tokens",
		stub("token", "Manage auth tokens",
			newAuthTokenCreateCmd(),
		),
	)
}

func newLogsCmd() *cobra.Command {
	return stubLeaf("logs", "Tail the local run log (use --follow to stream)")
}

func newAbilitiesCmd() *cobra.Command {
	return stub("abilities", "Work with discovered abilities",
		stubLeaf("refresh", "Re-discover abilities from connected stores"),
	)
}

func newExportCmd() *cobra.Command {
	return stubLeaf("export", "Export run data as JSONL (opt-in GEPA bundle)")
}
