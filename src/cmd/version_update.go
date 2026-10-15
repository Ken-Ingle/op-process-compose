package cmd

import (
	"github.com/Ken-Ingle/op-process-compose/src/updater"
	"github.com/spf13/cobra"
)

var versionUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update process-compose to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		return updater.Update()
	},
}

func init() {
	versionCmd.AddCommand(versionUpdateCmd)
}
