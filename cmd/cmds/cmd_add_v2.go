package cmds

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewAddCommandV2 creates the new service-based add command
func NewAddCommandV2() *cobra.Command {
	var (
		repos  []string
		branch string
		force  bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "add-v2 <workspace-name> [repo-names...]",
		Short: "Add repositories to an existing workspace (new architecture)",
		Long: `Add one or more repositories to an existing workspace using the new service architecture.
The command will create worktrees for the new repositories on the workspace's branch.

Examples:
  # Add single repository to workspace
  wsm add-v2 my-feature new-repo

  # Add multiple repositories to workspace
  wsm add-v2 my-feature repo1 repo2 repo3

  # Add repositories using --repos flag
  wsm add-v2 my-feature --repos repo1,repo2,repo3

  # Add repository with specific branch
  wsm add-v2 my-feature new-repo --branch feature/custom-branch

  # Preview changes without actually adding
  wsm add-v2 my-feature new-repo --dry-run

  # Force overwrite if branch already exists
  wsm add-v2 my-feature new-repo --force`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := args[0]

			// Combine positional args and --repos flag
			var allRepos []string
			if len(args) > 1 {
				allRepos = append(allRepos, args[1:]...)
			}
			allRepos = append(allRepos, repos...)

			if len(allRepos) == 0 {
				return errors.New("no repositories specified. Provide repository names as arguments or use --repos flag")
			}

			return runAddV2(cmd.Context(), workspaceName, allRepos, branch, force, dryRun)
		},
	}

	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repository names to add (comma-separated)")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name for worktrees (if not specified, uses workspace's branch)")
	cmd.Flags().BoolVar(&force, "force", false, "Force overwrite if branch already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be added without actually adding")

	return cmd
}

func runAddV2(ctx context.Context, workspaceName string, repoNames []string, branch string, force, dryRun bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Add repositories to workspace using the new service
	deps.Logger.Info("Adding repositories to workspace",
		ux.Field("workspace", workspaceName),
		ux.Field("repos", repoNames),
		ux.Field("branch", branch),
		ux.Field("force", force),
		ux.Field("dryRun", dryRun))

	workspace, err := workspaceService.AddRepositoriesToWorkspace(ctx, service.AddRepositoriesToWorkspaceRequest{
		WorkspaceName: workspaceName,
		RepoNames:     repoNames,
		Branch:        branch,
		Force:         force,
		DryRun:        dryRun,
	})

	if err != nil {
		// Check if user cancelled - handle gracefully without error
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "cancelled") || strings.Contains(errMsg, "interrupt") {
			deps.Logger.Info("Operation cancelled by user")
			return nil
		}
		return errors.Wrap(err, "failed to add repositories to workspace")
	}

	// Show results
	if dryRun {
		return showAddPreviewV2(*workspace, repoNames, deps.Logger)
	}

	// Success output
	output.PrintSuccess("Successfully added %d repositories to workspace '%s'!", len(repoNames), workspace.Name)
	fmt.Println()

	output.PrintHeader("Updated Workspace Details")
	fmt.Printf("  Name: %s\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)

	allRepoNames := make([]string, len(workspace.Repositories))
	for i, repo := range workspace.Repositories {
		allRepoNames[i] = repo.Name
	}
	fmt.Printf("  Total repositories: %s\n", strings.Join(allRepoNames, ", "))

	if workspace.Branch != "" {
		fmt.Printf("  Branch: %s\n", workspace.Branch)
	}
	if workspace.GoWorkspace {
		fmt.Printf("  Go workspace: yes (go.work updated)\n")
	}

	fmt.Println()
	output.PrintInfo("Added repositories:")
	for _, repoName := range repoNames {
		fmt.Printf("  - %s -> %s\n", repoName, workspace.RepositoryWorktreePath(repoName))
	}

	fmt.Println()
	output.PrintInfo("To start working:")
	fmt.Printf("  cd %s\n", workspace.Path)

	deps.Logger.Info("Repository addition completed successfully",
		ux.Field("workspace", workspace.Name),
		ux.Field("addedRepos", len(repoNames)))

	return nil
}

func showAddPreviewV2(workspace domain.Workspace, newRepoNames []string, logger ux.Logger) error {
	output.PrintHeader("Dry Run - Repository Addition Preview")
	fmt.Printf("  Workspace: %s\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)

	if workspace.Branch != "" {
		fmt.Printf("  Branch: %s\n", workspace.Branch)
	}

	fmt.Println()
	output.PrintInfo("Repositories that would be added:")
	for _, repoName := range newRepoNames {
		fmt.Printf("  - %s -> %s\n", repoName, workspace.RepositoryWorktreePath(repoName))
	}

	fmt.Println()
	output.PrintInfo("Files that would be updated:")

	// Show metadata file
	fmt.Printf("  %s (workspace metadata)\n", workspace.MetadataPath())

	// Show go.work if applicable
	if workspace.GoWorkspace {
		fmt.Printf("  %s (go workspace)\n", workspace.GoWorkPath())
	}

	// Show workspace config
	fmt.Printf("  %s (workspace configuration)\n", fmt.Sprintf("~/.config/workspace-manager/workspaces/%s.json", workspace.Name))

	return nil
}
