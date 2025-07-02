package cmds

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewForkCommandV2 creates the new service-based fork command
func NewForkCommandV2() *cobra.Command {
	var (
		branch       string
		branchPrefix string
		agentSource  string
		dryRun       bool
		workspace    string
	)

	cmd := &cobra.Command{
		Use:   "fork-v2 <new-workspace-name> [source-workspace-name]",
		Short: "Create a new workspace by forking an existing workspace (new architecture)",
		Long: `Create a new workspace that is a fork of an existing workspace using the new service architecture.
The new workspace will contain the same repositories as the source workspace,
with a new branch created from the current branch of the source workspace.

If no source workspace is specified, attempts to detect the current workspace.

The source workspace's current branch will be used as the base branch for 
the new workspace's branch.

Examples:
  # Fork current workspace to create "my-feature"
  wsm fork-v2 my-feature

  # Fork a specific workspace
  wsm fork-v2 my-feature source-workspace

  # Fork with custom branch name
  wsm fork-v2 my-feature --branch feature/new-api

  # Fork with custom branch prefix (bug/my-feature)
  wsm fork-v2 my-feature --branch-prefix bug`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			newWorkspaceName := args[0]
			sourceWorkspaceName := workspace
			if len(args) > 1 {
				sourceWorkspaceName = args[1]
			}
			return runForkV2(cmd.Context(), newWorkspaceName, sourceWorkspaceName, branch, branchPrefix, agentSource, dryRun)
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "", "Branch name for the new workspace (if not specified, uses <branch-prefix>/<new-workspace-name>)")
	cmd.Flags().StringVar(&branchPrefix, "branch-prefix", "task", "Prefix for auto-generated branch names")
	cmd.Flags().StringVar(&agentSource, "agent-source", "", "Path to AGENT.md template file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be created without actually creating")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Source workspace name")

	return cmd
}

func runForkV2(ctx context.Context, newWorkspaceName, sourceWorkspaceName, branch, branchPrefix, agentSource string, dryRun bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// If no source workspace specified, try to detect current workspace
	if sourceWorkspaceName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}

		detected, err := workspaceService.DetectWorkspace(cwd)
		if err != nil {
			return errors.Wrap(err, "failed to detect workspace. Use 'wsm fork-v2 <new-name> <source-workspace>' or specify --workspace flag")
		}
		sourceWorkspaceName = detected
	}

	// Fork the workspace using the new service
	deps.Logger.Info("Forking workspace",
		ux.Field("source", sourceWorkspaceName),
		ux.Field("new", newWorkspaceName),
		ux.Field("branch", branch),
		ux.Field("branchPrefix", branchPrefix),
		ux.Field("dryRun", dryRun))

	workspace, err := workspaceService.ForkWorkspace(ctx, service.ForkRequest{
		NewWorkspaceName:    newWorkspaceName,
		SourceWorkspaceName: sourceWorkspaceName,
		Branch:              branch,
		BranchPrefix:        branchPrefix,
		AgentSource:         agentSource,
		DryRun:              dryRun,
	})

	if err != nil {
		// Check if user cancelled - handle gracefully without error
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "cancelled") || strings.Contains(errMsg, "interrupt") {
			deps.Logger.Info("Operation cancelled by user")
			return nil
		}
		return errors.Wrap(err, "failed to fork workspace")
	}

	// Show results
	if dryRun {
		return showForkPreviewV2(*workspace, sourceWorkspaceName, deps.Logger)
	}

	// Success output
	output.PrintSuccess("Workspace '%s' forked successfully from '%s'!", workspace.Name, sourceWorkspaceName)
	fmt.Println()

	output.PrintHeader("Fork Details")
	fmt.Printf("  Source: %s (branch: %s)\n", sourceWorkspaceName, workspace.BaseBranch)
	fmt.Printf("  New workspace: %s\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)

	repoNames := make([]string, len(workspace.Repositories))
	for i, repo := range workspace.Repositories {
		repoNames[i] = repo.Name
	}
	fmt.Printf("  Repositories: %s\n", strings.Join(repoNames, ", "))

	fmt.Printf("  New branch: %s\n", workspace.Branch)
	fmt.Printf("  Base branch: %s\n", workspace.BaseBranch)
	if workspace.GoWorkspace {
		fmt.Printf("  Go workspace: yes (go.work created)\n")
	}
	if workspace.AgentMD != "" {
		fmt.Printf("  AGENT.md: copied from source workspace\n")
	}

	fmt.Println()
	output.PrintInfo("To start working:")
	fmt.Printf("  cd %s\n", workspace.Path)

	deps.Logger.Info("Workspace fork completed successfully",
		ux.Field("source", sourceWorkspaceName),
		ux.Field("new", workspace.Name),
		ux.Field("path", workspace.Path))

	return nil
}

func showForkPreviewV2(workspace domain.Workspace, sourceWorkspaceName string, logger ux.Logger) error {
	output.PrintHeader("📋 Fork Preview: %s → %s", sourceWorkspaceName, workspace.Name)
	fmt.Println()

	fmt.Printf("  Source workspace: %s\n", sourceWorkspaceName)
	fmt.Printf("  New workspace: %s\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)

	repoNames := make([]string, len(workspace.Repositories))
	for i, repo := range workspace.Repositories {
		repoNames[i] = repo.Name
	}
	fmt.Printf("  Repositories: %s\n", strings.Join(repoNames, ", "))

	if workspace.Branch != "" {
		fmt.Printf("  New branch: %s\n", workspace.Branch)
	}
	if workspace.BaseBranch != "" {
		fmt.Printf("  Base branch: %s\n", workspace.BaseBranch)
	}
	if workspace.GoWorkspace {
		fmt.Printf("  Go workspace: yes (go.work would be created)\n")
	}
	if workspace.AgentMD != "" {
		fmt.Printf("  AGENT.md: would be copied from source workspace\n")
	}

	fmt.Println()
	output.PrintInfo("Files that would be created:")

	// Show metadata file
	fmt.Printf("  %s\n", workspace.MetadataPath())

	// Show go.work if applicable
	if workspace.GoWorkspace {
		fmt.Printf("  %s\n", workspace.GoWorkPath())
	}

	// Show AGENT.md if applicable
	if workspace.AgentMD != "" {
		fmt.Printf("  %s\n", workspace.AgentMDPath())
	}

	// Show worktree paths
	fmt.Println()
	output.PrintInfo("Worktrees that would be created:")
	for _, repo := range workspace.Repositories {
		fmt.Printf("  %s -> %s\n", repo.Name, workspace.RepositoryWorktreePath(repo.Name))
	}

	return nil
}
