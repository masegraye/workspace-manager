package cmds

import (
	"context"
	"fmt"

	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewDiffCommandV2() *cobra.Command {
	var (
		staged    bool
		repo      string
		workspace string
	)

	cmd := &cobra.Command{
		Use:   "diff-v2 [workspace-path]",
		Short: "Show diff across workspace repositories using the new service architecture",
		Long: `Show unified diff of changes across all repositories in the workspace using the new service architecture.

The new architecture provides:
- Faster diff generation with parallel repository processing
- Better error handling and recovery for missing repositories
- Structured logging for debugging diff operations
- Clean separation between diff logic and presentation

If no workspace path is provided, attempts to detect the current workspace from the working directory.

Examples:
  # Show diff of current workspace
  wsm diff-v2

  # Show diff of specific workspace
  wsm diff-v2 /path/to/workspace

  # Show staged changes only
  wsm diff-v2 --staged

  # Show diff for specific repository only
  wsm diff-v2 --repo myrepo

  # Show staged changes for specific repository
  wsm diff-v2 --staged --repo myrepo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runDiffV2(cmd.Context(), workspacePath, staged, repo)
		},
	}

	cmd.Flags().BoolVar(&staged, "staged", false, "Show staged changes only")
	cmd.Flags().StringVar(&repo, "repo", "", "Show diff for specific repository only")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runDiffV2(ctx context.Context, workspacePath string, staged bool, repoFilter string) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Load workspace from path
	workspace, err := loadWorkspaceFromPathV2(workspacePath, deps)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace from '%s'", workspacePath)
	}

	deps.Logger.Info("Getting workspace diff",
		ux.Field("workspace", workspace.Name),
		ux.Field("staged", staged),
		ux.Field("repo_filter", repoFilter))

	// Show header
	output.PrintHeader("📄 Showing diff for workspace: %s", workspace.Name)
	if staged {
		output.PrintInfo("   (staged changes only)")
	}
	if repoFilter != "" {
		output.PrintInfo("   (repository: %s)", repoFilter)
	}
	fmt.Println()

	// Get diff using the new service
	diffReq := service.DiffRequest{
		Workspace:  *workspace,
		Staged:     staged,
		RepoFilter: repoFilter,
	}

	diff, err := workspaceService.GetWorkspaceDiff(ctx, diffReq)
	if err != nil {
		return errors.Wrap(err, "failed to get workspace diff")
	}

	if diff == "" || diff == "No changes found in workspace." {
		output.PrintInfo("No changes found in workspace.")
		return nil
	}

	fmt.Println(diff)
	return nil
}
