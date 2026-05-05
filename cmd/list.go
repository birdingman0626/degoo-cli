package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	flagListLong      bool
	flagListRecursive bool
)

var listCmd = &cobra.Command{
	Use:   "list [remote-path]",
	Short: "List files and folders at a remote Degoo path",
	Long:  "List files and folders at a remote Degoo path. Defaults to the root directory (\"/\").",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		remotePath := "/"
		if len(args) == 1 {
			remotePath = args[0]
		}

		folderID, err := apiClient.ResolveRemotePath(remotePath)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", remotePath, err)
		}

		return listDir(folderID, remotePath, 0)
	},
}

func listDir(folderID, displayPath string, depth int) error {
	children, err := apiClient.GetChildren(folderID)
	if err != nil {
		return fmt.Errorf("list %s: %w", displayPath, err)
	}

	indent := strings.Repeat("  ", depth)
	for _, item := range children {
		if flagListLong {
			kind := "file"
			if item.IsDirectory {
				kind = "dir "
			}
			mtime := ""
			if !item.ModifiedTime.IsZero() {
				mtime = item.ModifiedTime.Format("2006-01-02 15:04")
			}
			fmt.Printf("%s%s  %-8s  %12s  %s\n",
				indent, kind, mtime, formatSize(item.Size), item.Name)
		} else {
			prefix := ""
			if item.IsDirectory {
				prefix = "[dir] "
			}
			fmt.Printf("%s%s%s\n", indent, prefix, item.Name)
		}

		if flagListRecursive && item.IsDirectory {
			subPath := displayPath + "/" + item.Name
			if err := listDir(item.ID, subPath, depth+1); err != nil {
				fmt.Printf("%s  (error listing %s: %v)\n", indent, item.Name, err)
			}
		}
	}
	return nil
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func init() {
	listCmd.Flags().BoolVarP(&flagListLong, "long", "l", false, "show type, date, size, and name")
	listCmd.Flags().BoolVarP(&flagListRecursive, "recursive", "r", false, "list subdirectories recursively")
	rootCmd.AddCommand(listCmd)
}
