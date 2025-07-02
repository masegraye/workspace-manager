package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewDeleteCommandV2 creates the new service-based delete command
func NewDeleteCommandV2() *cobra.Command {
	var (
		force          bool
		forceWorktrees bool
		removeFiles    bool
		outputFormat   string
	)

	cmd := &cobra.Command{
		Use:   "delete-v2 <workspace-name>",
		Short: "Delete a workspace (new architecture)",
		Long: `Delete a workspace and optionally remove its files using the new service architecture.

This command removes the workspace configuration and optionally deletes
the workspace directory and all its contents. Use with caution.

Examples:
  # Delete workspace configuration only
  wsm delete-v2 my-workspace

  # Delete workspace and all files
  wsm delete-v2 my-workspace --remove-files

  # Force delete without confirmation
  wsm delete-v2 my-workspace --force --remove-files

  # Force worktree removal even with uncommitted changes
  wsm delete-v2 my-workspace --force-worktrees --remove-files`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteV2(cmd.Context(), args[0], force, forceWorktrees, removeFiles, outputFormat)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force delete without confirmation")
	cmd.Flags().BoolVar(&forceWorktrees, "force-worktrees", false, "Force worktree removal even with uncommitted changes")
	cmd.Flags().BoolVar(&removeFiles, "remove-files", false, "Remove workspace files and directories")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json)")

	return cmd
}

func runDeleteV2(ctx context.Context, workspaceName string, force bool, forceWorktrees bool, removeFiles bool, outputFormat string) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Load workspace
	workspace, err := workspaceService.LoadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "workspace '%s' not found", workspaceName)
	}

	// Show workspace status first
	fmt.Printf("Current workspace status for '%s':\n", workspace.Name)
	status, err := workspaceService.GetWorkspaceStatus(ctx, *workspace)
	if err == nil {
		// Print basic status info
		fmt.Printf("  Path: %s\n", workspace.Path)
		fmt.Printf("  Repositories: %d\n", len(workspace.Repositories))
		fmt.Printf("  Branch: %s\n", workspace.Branch)
		fmt.Printf("  Status: %s\n", getStatusSummary(status))
	} else {
		deps.Logger.Error("Error getting status", ux.Field("error", err))
	}
	fmt.Printf("\n")

	// Show what will be deleted
	if outputFormat == "json" {
		data, err := json.MarshalIndent(workspace, "", "  ")
		if err != nil {
			return errors.Wrap(err, "failed to marshal workspace to JSON")
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Workspace: %s\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)
	fmt.Printf("  Repositories: %d\n", len(workspace.Repositories))

	fmt.Printf("\nThis will:\n")
	if forceWorktrees {
		fmt.Printf("  1. Remove git worktrees (git worktree remove --force)\n")
	} else {
		fmt.Printf("  1. Remove git worktrees (git worktree remove)\n")
		fmt.Printf("     ⚠️  Will fail if there are uncommitted changes\n")
	}

	if removeFiles {
		fmt.Printf("  2. 🚨 DELETE the workspace directory and ALL its contents!\n")
		fmt.Printf("     📁 This includes: go.work, AGENT.md, and all repository worktrees\n")
	} else {
		fmt.Printf("  2. Remove workspace configuration\n")
		fmt.Printf("  3. Clean up workspace-specific files (go.work, AGENT.md)\n")
		fmt.Printf("  4. Repository worktrees will remain at: %s\n", workspace.Path)
	}

	// Confirm deletion unless forced
	if !force {
		confirmed, err := deps.Prompter.Confirm(
			fmt.Sprintf("Are you sure you want to delete workspace '%s'? This action cannot be undone.", workspaceName),
		)
		if err != nil {
			// Check if user cancelled
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "user aborted") ||
				strings.Contains(errMsg, "cancelled") ||
				strings.Contains(errMsg, "aborted") ||
				strings.Contains(errMsg, "interrupt") {
				deps.Logger.Info("Operation cancelled")
				return nil
			}
			return errors.Wrap(err, "confirmation failed")
		}

		if !confirmed {
			deps.Logger.Info("Operation cancelled")
			return nil
		}
	}

	// Perform deletion
	if err := workspaceService.DeleteWorkspace(ctx, workspaceName, removeFiles, forceWorktrees); err != nil {
		return errors.Wrap(err, "failed to delete workspace")
	}

	if removeFiles {
		deps.Logger.Info("Workspace and all files deleted successfully", ux.Field("workspace", workspaceName))
	} else {
		deps.Logger.Info("Workspace configuration deleted successfully", ux.Field("workspace", workspaceName))
		fmt.Printf("Files remain at: %s\n", workspace.Path)
	}

	return nil
}

// getStatusSummary returns a summary of the workspace status
func getStatusSummary(status interface{}) string {
	// This is a placeholder - in a real implementation you'd inspect the status
	// For now, just return a generic message
	return "Status information available"
}
