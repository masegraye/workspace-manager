package cmds

import (
	"context"
	"fmt"
	"os"

	"github.com/carapace-sh/carapace"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewPathCommand() *cobra.Command {
	var workspace string

	cmd := &cobra.Command{
		Use:   "path [workspace-name]",
		Short: "Get workspace path",
		Long: `Output the full path to a workspace.

Useful for shell integration. If no workspace is specified, attempts to detect
the current workspace from your current directory.

Examples:
  # Get path of a specific workspace
  wsm path my-workspace

  # Use with cd command
  cd $(wsm path my-workspace)

  # Detect current workspace path
  wsm path`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := workspace
			if len(args) > 0 {
				workspaceName = args[0]
			}
			return runPath(cmd.Context(), workspaceName)
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name")
	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runPath(ctx context.Context, workspaceName string) error {
	// If no workspace specified, try to detect current workspace
	if workspaceName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}

		detected, err := detectWorkspace(cwd)
		if err != nil {
			return errors.Wrap(err, "failed to detect workspace. Use 'wsm path <workspace-name>' or specify --workspace flag")
		}
		workspaceName = detected
	}

	// Load workspace
	workspace, err := loadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "workspace '%s' not found", workspaceName)
	}

	// Output just the path (clean output for shell integration)
	fmt.Println(workspace.Path)
	return nil
}
