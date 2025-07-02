package cmds

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/sync"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewBranchCommand creates the new service-based branch command
func NewBranchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Manage branches across workspace repositories",
		Long: `Create, switch, and manage branches across all repositories in the workspace.
This ensures consistent branch operations across your multi-repository development.

Examples:
  # Create a new branch across all repositories
  wsm branch create feature/new-api

  # Create a new branch with tracking
  wsm branch create feature/new-api --track

  # Switch to an existing branch across all repositories
  wsm branch switch main

  # List current branches across all repositories
  wsm branch list`,
	}

	cmd.AddCommand(
		NewBranchCreateCommand(),
		NewBranchSwitchCommand(),
		NewBranchListCommand(),
	)

	return cmd
}

func NewBranchCreateCommand() *cobra.Command {
	var track bool

	cmd := &cobra.Command{
		Use:   "create [branch-name]",
		Short: "Create a branch across all repositories",
		Long:  "Create a new branch with the same name across all repositories in the workspace.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchCreateV2(cmd.Context(), args[0], track)
		},
	}

	cmd.Flags().BoolVar(&track, "track", false, "Set up tracking for the new branch")

	return cmd
}

func NewBranchSwitchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch [branch-name]",
		Short: "Switch to a branch across all repositories",
		Long:  "Switch all repositories in the workspace to the specified branch.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchSwitchV2(cmd.Context(), args[0])
		},
	}

	return cmd
}

func NewBranchListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List current branches across repositories",
		Long:  "Show the current branch for each repository in the workspace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranchListV2(cmd.Context())
		},
	}

	return cmd
}

func runBranchCreateV2(ctx context.Context, branchName string, track bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Detect current workspace
	cwd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "failed to get current directory")
	}

	workspaceName, err := workspaceService.DetectWorkspace(cwd)
	if err != nil {
		return errors.Wrap(err, "failed to detect current workspace")
	}

	// Load workspace
	workspace, err := workspaceService.LoadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace '%s'", workspaceName)
	}

	deps.Logger.Info("Creating branch across workspace",
		ux.Field("workspace", workspace.Name),
		ux.Field("branch", branchName),
		ux.Field("track", track))

	output.PrintHeader("🌿 Creating branch '%s' across workspace: %s", branchName, workspace.Name)

	// Create branch using workspace service
	results, err := workspaceService.CreateBranchWorkspace(ctx, *workspace, branchName, track)
	if err != nil {
		return errors.Wrap(err, "branch creation failed")
	}

	return printBranchResultsV2(results, "create")
}

func runBranchSwitchV2(ctx context.Context, branchName string) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Detect current workspace
	cwd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "failed to get current directory")
	}

	workspaceName, err := workspaceService.DetectWorkspace(cwd)
	if err != nil {
		return errors.Wrap(err, "failed to detect current workspace")
	}

	// Load workspace
	workspace, err := workspaceService.LoadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace '%s'", workspaceName)
	}

	deps.Logger.Info("Switching branch across workspace",
		ux.Field("workspace", workspace.Name),
		ux.Field("branch", branchName))

	output.PrintHeader("🔄 Switching to branch '%s' across workspace: %s", branchName, workspace.Name)

	// Switch branch using workspace service
	results, err := workspaceService.SwitchBranchWorkspace(ctx, *workspace, branchName)
	if err != nil {
		return errors.Wrap(err, "branch switch failed")
	}

	return printBranchResultsV2(results, "switch")
}

func runBranchListV2(ctx context.Context) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Detect current workspace
	cwd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "failed to get current directory")
	}

	workspaceName, err := workspaceService.DetectWorkspace(cwd)
	if err != nil {
		return errors.Wrap(err, "failed to detect current workspace")
	}

	// Load workspace
	workspace, err := workspaceService.LoadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace '%s'", workspaceName)
	}

	deps.Logger.Info("Listing branches across workspace", ux.Field("workspace", workspace.Name))

	output.PrintHeader("📋 Current branches in workspace: %s", workspace.Name)

	// Get branch information using workspace service
	results, err := workspaceService.ListBranchesWorkspace(ctx, *workspace)
	if err != nil {
		return errors.Wrap(err, "failed to list branches")
	}

	// Get status for each repository to show additional context
	status, err := workspaceService.GetWorkspaceStatus(ctx, *workspace)
	if err != nil {
		deps.Logger.Warn("Failed to get workspace status", ux.Field("error", err))
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			deps.Logger.Warn("Failed to flush table writer", ux.Field("error", err))
		}
	}()

	fmt.Fprintln(w, "\nREPOSITORY\tCURRENT BRANCH\tSTATUS")
	fmt.Fprintln(w, "----------\t--------------\t------")

	for _, result := range results {
		statusSymbol := "✅"
		if !result.Success {
			statusSymbol = "❌"
		} else if status != nil {
			// Find matching repository status
			for _, repoStatus := range status.Repositories {
				if repoStatus.Repository.Name == result.Repository {
					if repoStatus.HasChanges {
						statusSymbol = "🔄"
					}
					if repoStatus.HasConflicts {
						statusSymbol = "⚠️"
					}
					break
				}
			}
		}

		branch := result.Branch
		if !result.Success {
			branch = "unknown"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n",
			result.Repository,
			branch,
			statusSymbol,
		)
	}

	fmt.Fprintln(w)
	return nil
}

func printBranchResultsV2(results []sync.BranchResult, operation string) error {
	if len(results) == 0 {
		output.PrintInfo("No repositories found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			output.LogWarn(
				fmt.Sprintf("Failed to flush table writer: %v", err),
				"Failed to flush table writer",
				"error", err,
			)
		}
	}()

	fmt.Fprintln(w, "\nREPOSITORY\tSTATUS\tERROR")
	fmt.Fprintln(w, "----------\t------\t-----")

	successCount := 0

	for _, result := range results {
		status := "✅"
		if !result.Success {
			status = "❌"
		} else {
			successCount++
		}

		errorMsg := result.Error
		if len(errorMsg) > 50 {
			errorMsg = errorMsg[:47] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n",
			result.Repository,
			status,
			errorMsg,
		)
	}

	fmt.Fprintln(w)

	// Summary
	output.PrintSuccess("Summary: %d/%d repositories %s successfully", successCount, len(results), operation)

	if successCount < len(results) {
		output.PrintWarning("Some repositories failed. Check errors above and resolve manually.")
	}

	return nil
}
