package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	syncer "degoo-cli/internal/sync"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <local-path> <remote-path>",
	Short: "Upload a local file or directory to Degoo",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		stats, err := syncer.Upload(apiClient, log, args[0], args[1])
		if err != nil {
			return err
		}
		printSummary("upload", stats)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)
}

func printSummary(op string, stats *syncer.Stats) {
	fmt.Println("=== Transfer Summary ===")
	if op == "upload" {
		fmt.Printf("Uploaded:  %d files (%.1f MB)\n", stats.Uploaded, float64(stats.Bytes)/1024/1024)
	} else {
		fmt.Printf("Downloaded: %d files (%.1f MB)\n", stats.Downloaded, float64(stats.Bytes)/1024/1024)
	}
	fmt.Printf("Skipped:   %d files (already up to date)\n", stats.Skipped)
	fmt.Printf("Failed:    %d files\n", len(stats.Failed))
	for _, f := range stats.Failed {
		fmt.Printf("  - %s  (max retries exceeded)\n", f)
	}
	fmt.Println("=======================")
}
