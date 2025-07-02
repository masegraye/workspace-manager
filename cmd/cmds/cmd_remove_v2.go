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

// NewRemoveCommandV2 creates the new service-based remove command
func NewRemoveCommandV2() *cobra.Command {
	var (
		force        bool
		removeFiles  bool
		outputFormat string
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:   "remove-v2 <workspace-name> <repo-name> [repo-name...]",
		Short: "Remove repositories from a workspace (new architecture)",
		Long: `Remove repositories from an existing workspace using the new service architecture.

This command removes repositories from a workspace by:
- Removing git worktrees for the specified repositories
- Updating the workspace configuration to exclude the repositories
- Updating go.work file if the workspace has Go repositories
- Optionally removing repository directories from the workspace

Examples:
  # Remove a single repository from a workspace
  wsm remove-v2 my-feature old-repo

  # Remove multiple repositories from a workspace
  wsm remove-v2 my-feature repo1 repo2 repo3

  # Force remove repositories (removes worktrees even with uncommitted changes)
  wsm remove-v2 my-feature old-repo --force

  # Remove repositories and their directories from workspace
  wsm remove-v2 my-feature old-repo --remove-files

  # Dry run to see what would be removed
  wsm remove-v2 my-feature old-repo --dry-run`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := args[0]
			repoNames := args[1:]
			return runRemoveV2(cmd.Context(), workspaceName, repoNames, force, removeFiles, outputFormat, dryRun)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force remove worktrees even with uncommitted changes")
	cmd.Flags().BoolVar(&removeFiles, "remove-files", false, "Remove repository directories from workspace")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be removed without making changes")

	return cmd
}

func runRemoveV2(ctx context.Context, workspaceName string, repoNames []string, force bool, removeFiles bool, outputFormat string, dryRun bool) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Load workspace
	workspace, err := workspaceService.LoadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "workspace '%s' not found", workspaceName)
	}

	// Show current workspace status
	fmt.Printf("Current workspace: '%s'\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)
	fmt.Printf("  Total repositories: %d\n", len(workspace.Repositories))

	// Find repositories to remove and validate they exist
	var toRemove []string
	var notFound []string

	for _, repoName := range repoNames {
		found := false
		for _, repo := range workspace.Repositories {
			if repo.Name == repoName {
				toRemove = append(toRemove, repoName)
				found = true
				break
			}
		}
		if !found {
			notFound = append(notFound, repoName)
		}
	}

	if len(notFound) > 0 {
		return errors.Errorf("repositories not found in workspace '%s': %v", workspaceName, notFound)
	}

	// Show what will be removed
	if outputFormat == "json" {
		data := map[string]interface{}{
			"workspace":   workspaceName,
			"toRemove":    toRemove,
			"force":       force,
			"removeFiles": removeFiles,
			"dryRun":      dryRun,
		}
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return errors.Wrap(err, "failed to marshal data to JSON")
		}
		fmt.Println(string(jsonData))
		if dryRun {
			return nil
		}
	}

	fmt.Printf("\nRepositories to remove: %v\n", toRemove)
	fmt.Printf("Remaining repositories: %d\n", len(workspace.Repositories)-len(toRemove))

	fmt.Printf("\nThis will:\n")
	if force {
		fmt.Printf("  1. Remove git worktrees (git worktree remove --force)\n")
	} else {
		fmt.Printf("  1. Remove git worktrees (git worktree remove)\n")
		fmt.Printf("     ⚠️  Will fail if there are uncommitted changes\n")
	}

	fmt.Printf("  2. Update workspace configuration\n")
	fmt.Printf("  3. Update go.work file if needed\n")

	if removeFiles {
		fmt.Printf("  4. 🚨 DELETE repository directories from workspace!\n")
		for _, repoName := range toRemove {
			fmt.Printf("     📁 %s/%s\n", workspace.Path, repoName)
		}
	} else {
		fmt.Printf("  4. Repository worktrees will be removed but directories may remain\n")
	}

	if dryRun {
		fmt.Printf("\n🔍 DRY RUN MODE - No changes will be made\n")
		return nil
	}

	// Confirm removal unless forced
	if !force || len(toRemove) > 1 {
		confirmed, err := deps.Prompter.Confirm(
			fmt.Sprintf("Are you sure you want to remove %d repositories from workspace '%s'?", len(toRemove), workspaceName),
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

	// Perform removal
	updatedWorkspace, err := workspaceService.RemoveRepositoriesFromWorkspace(ctx, service.RemoveRepositoriesFromWorkspaceRequest{
		WorkspaceName: workspaceName,
		RepoNames:     toRemove,
		Force:         force,
		RemoveFiles:   removeFiles,
		DryRun:        false,
	})
	if err != nil {
		return errors.Wrap(err, "failed to remove repositories from workspace")
	}

	fmt.Printf("\n✓ Successfully removed %d repositories from workspace '%s'\n", len(toRemove), workspaceName)
	fmt.Printf("  Remaining repositories: %d\n", len(updatedWorkspace.Repositories))

	if len(updatedWorkspace.Repositories) == 0 {
		fmt.Printf("  💡 Workspace is now empty. Consider deleting it with: wsm delete-v2 %s\n", workspaceName)
	}

	deps.Logger.Info("Repositories removed successfully",
		ux.Field("workspace", workspaceName),
		ux.Field("removed", toRemove))

	return nil
}
