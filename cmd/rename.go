package cmd

import (
	"fmt"
	"path"
	"strings"

	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename <remote-path> <new-name>",
	Short: "Rename a file or folder on Degoo",
	Long:  "Rename a file or folder at the given remote path to a new name (no path separators).",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		remotePath := args[0]
		newName := args[1]

		if strings.ContainsAny(newName, "/\\") {
			return fmt.Errorf("new-name must be a bare filename, not a path")
		}

		parent := path.Dir(remotePath)
		oldName := path.Base(remotePath)

		parentID, err := apiClient.ResolveRemotePath(parent)
		if err != nil {
			return fmt.Errorf("resolve parent %s: %w", parent, err)
		}

		children, err := apiClient.GetChildren(parentID)
		if err != nil {
			return fmt.Errorf("list %s: %w", parent, err)
		}

		var fileID string
		for _, child := range children {
			if strings.EqualFold(child.Name, oldName) {
				fileID = child.ID
				break
			}
		}
		if fileID == "" {
			return fmt.Errorf("%q not found in %s", oldName, parent)
		}

		if err := apiClient.RenameFile(fileID, newName); err != nil {
			return err
		}
		fmt.Printf("Renamed: %s → %s\n", oldName, newName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(renameCmd)
}
