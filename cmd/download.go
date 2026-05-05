package cmd

import (
	"github.com/spf13/cobra"

	syncer "degoo-cli/internal/sync"
)

var downloadCmd = &cobra.Command{
	Use:   "download <remote-path> <local-path>",
	Short: "Download a Degoo file or directory to local storage",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		stats, err := syncer.Download(apiClient, log, args[0], args[1])
		if err != nil {
			return err
		}
		printSummary("download", stats)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)
}
