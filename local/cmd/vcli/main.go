package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vultisig/vcli/local/cmd/vcli/cmd"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "vcli",
		Short: "Vultisig CLI for local plugin development and testing",
		Long: `vcli is a CLI tool for testing Vultisig plugins locally.

This tool allows you to skip the browser extension and plugin marketplace UI
to quickly test any plugin you want to build.

PLUGIN ALIASES (use short names instead of full IDs):
  dca    -> vultisig-dca-0000        (Recurring Swaps)
  fee    -> vultisig-fees-feee       (Fee collection)
  sends  -> vultisig-recurring-sends-0000 (Recurring Sends)

  Run 'vcli plugin aliases' for full list.

ENVIRONMENT VARIABLES:
  VAULT_PASSWORD  - Default password for all TSS operations
  VAULT_PATH      - Default vault file path for import

  Tip: Put these in local/vault.env - vcli.sh auto-loads it!

QUICK START:
  1. vcli start                                                      # Start services
  2. vcli vault import --file vault.vult --password xxx              # Import vault
  3. vcli plugin install dca --password xxx                          # Install plugin (4-party TSS)
  4. vcli policy add --plugin dca --policy-file policy.json --password xxx
  5. vcli report                                                     # Check status
  6. vcli stop                                                       # Stop services

FLAG CONVENTIONS:
  --password  = Vault/Fast Vault password (all commands)
  --plugin    = Plugin ID or alias
  -c, --policy-file    = Config file path

Commands:
  start    - Start all local development services
  stop     - Stop all local development services
  vault    - Import, list, and manage vaults
  plugin   - List, install, and manage plugins (use 'plugin aliases' for short names)
  policy   - Create and manage policies
  auth     - Authenticate with verifier using TSS keysign
  verify   - Check transaction history and service health
  report   - Show comprehensive validation report
  status   - Show quick service status
`,
	}

	rootCmd.AddCommand(cmd.NewStartCmd())
	rootCmd.AddCommand(cmd.NewStopCmd())
	rootCmd.AddCommand(cmd.NewVaultCmd())
	rootCmd.AddCommand(cmd.NewPluginCmd())
	rootCmd.AddCommand(cmd.NewPolicyCmd())
	rootCmd.AddCommand(cmd.NewServicesCmd())
	rootCmd.AddCommand(cmd.NewStatusCmd())
	rootCmd.AddCommand(cmd.NewAuthCmd())
	rootCmd.AddCommand(cmd.NewVerifyCmd())
	rootCmd.AddCommand(cmd.NewReportCmd())
	rootCmd.AddCommand(cmd.NewDevTokenCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
