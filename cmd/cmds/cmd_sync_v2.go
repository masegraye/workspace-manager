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

func NewSyncCommandV2() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync-v2",
		Short: "Synchronize workspace repositories using new service architecture",
		Long: `Synchronize all repositories in the workspace with their remotes using the new service architecture.

The new architecture provides:
- Parallel sync operations for faster execution
- Better error handling and rollback capabilities
- Detailed progress reporting with structured logging
- Safe operations with conflict detection and resolution guidance

Supports pulling latest changes, pushing local commits, and fetching updates.`,
	}

	cmd.AddCommand(
		NewSyncAllCommandV2(),
		NewSyncPullCommandV2(),
		NewSyncPushCommandV2(),
		NewSyncFetchCommandV2(),
	)

	return cmd
}

func NewSyncAllCommandV2() *cobra.Command {
	var (
		pull   bool
		push   bool
		rebase bool
		dryRun bool
		workspace string
	)

	cmd := &cobra.Command{
		Use:   "all [workspace-path]",
		Short: "Sync all repositories (pull and push) using new architecture",
		Long: `Synchronize all repositories by pulling latest changes and pushing local commits.

Examples:
  # Sync current workspace (pull and push)
  wsm sync-v2 all

  # Only pull changes
  wsm sync-v2 all --pull --no-push

  # Only push changes  
  wsm sync-v2 all --no-pull --push

  # Use rebase instead of merge when pulling
  wsm sync-v2 all --rebase

  # Dry run to see what would be done
  wsm sync-v2 all --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runSyncAllV2(cmd.Context(), workspacePath, pull, push, rebase, dryRun)
		},
	}

	cmd.Flags().BoolVar(&pull, "pull", true, "Pull latest changes")
	cmd.Flags().BoolVar(&push, "push", true, "Push local commits")
	cmd.Flags().BoolVar(&rebase, "rebase", false, "Use rebase when pulling")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")

	return cmd
}

func NewSyncPullCommandV2() *cobra.Command {
	var (
		rebase bool
		dryRun bool
		workspace string
	)

	cmd := &cobra.Command{
		Use:   "pull [workspace-path]",
		Short: "Pull latest changes from all repositories using new architecture",
		Long: `Pull latest changes from remote repositories in the workspace.

Examples:
  # Pull changes for current workspace
  wsm sync-v2 pull

  # Pull with rebase instead of merge
  wsm sync-v2 pull --rebase

  # Dry run to see what would be pulled
  wsm sync-v2 pull --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runSyncPullV2(cmd.Context(), workspacePath, rebase, dryRun)
		},
	}

	cmd.Flags().BoolVar(&rebase, "rebase", false, "Use rebase instead of merge")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")

	return cmd
}

func NewSyncPushCommandV2() *cobra.Command {
	var (
		dryRun bool
		workspace string
	)

	cmd := &cobra.Command{
		Use:   "push [workspace-path]",
		Short: "Push local commits from all repositories using new architecture",
		Long: `Push local commits to remote repositories in the workspace.

Examples:
  # Push changes for current workspace
  wsm sync-v2 push

  # Dry run to see what would be pushed
  wsm sync-v2 push --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runSyncPushV2(cmd.Context(), workspacePath, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")

	return cmd
}

func NewSyncFetchCommandV2() *cobra.Command {
	var workspace string

	cmd := &cobra.Command{
		Use:   "fetch [workspace-path]",
		Short: "Fetch updates from all repositories without merging using new architecture",
		Long: `Fetch updates from remote repositories without merging them into the working branches.
This is useful to see what changes are available without modifying your working directories.

Examples:
  # Fetch updates for current workspace
  wsm sync-v2 fetch

  # Fetch updates for specific workspace
  wsm sync-v2 fetch /path/to/workspace`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runSyncFetchV2(cmd.Context(), workspacePath)
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")

	return cmd
}

// Implementation functions

func runSyncAllV2(ctx context.Context, workspacePath string, pull, push, rebase, dryRun bool) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Load workspace
	workspace, err := loadWorkspaceFromPathV2(workspacePath, deps)
	if err != nil {
		return err
	}

	deps.Logger.Info("Starting workspace sync", 
		ux.Field("workspace", workspace.Name),
		ux.Field("pull", pull),
		ux.Field("push", push),
		ux.Field("rebase", rebase),
		ux.Field("dryRun", dryRun))

	// Perform sync
	results, err := workspaceService.SyncWorkspace(ctx, *workspace, sync.SyncOptions{
		Pull:   pull,
		Push:   push,
		Rebase: rebase,
		DryRun: dryRun,
	})
	if err != nil {
		return errors.Wrap(err, "sync operation failed")
	}

	// Display results
	return displaySyncResults(results, dryRun)
}

func runSyncPullV2(ctx context.Context, workspacePath string, rebase, dryRun bool) error {
	return runSyncAllV2(ctx, workspacePath, true, false, rebase, dryRun)
}

func runSyncPushV2(ctx context.Context, workspacePath string, dryRun bool) error {
	return runSyncAllV2(ctx, workspacePath, false, true, false, dryRun)
}

func runSyncFetchV2(ctx context.Context, workspacePath string) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Load workspace
	workspace, err := loadWorkspaceFromPathV2(workspacePath, deps)
	if err != nil {
		return err
	}

	deps.Logger.Info("Starting workspace fetch", ux.Field("workspace", workspace.Name))

	// Perform fetch
	err = workspaceService.FetchWorkspace(ctx, *workspace)
	if err != nil {
		return errors.Wrap(err, "fetch operation failed")
	}

	output.PrintSuccess("Fetch completed for workspace '%s'", workspace.Name)
	output.PrintInfo("Use 'wsm status-v2' to see what changes are available")

	return nil
}

func displaySyncResults(results []sync.SyncResult, dryRun bool) error {
	if dryRun {
		output.PrintHeader("Sync Results (Dry Run)")
	} else {
		output.PrintHeader("Sync Results")
	}

	if len(results) == 0 {
		output.PrintInfo("No repositories to sync")
		return nil
	}

	// Count results
	successful := 0
	failed := 0
	pulled := 0
	pushed := 0
	conflicts := 0

	for _, result := range results {
		if result.Success {
			successful++
		} else {
			failed++
		}
		if result.Pulled {
			pulled++
		}
		if result.Pushed {
			pushed++
		}
		if result.Conflicts {
			conflicts++
		}
	}

	// Summary
	fmt.Printf("Summary: %d successful, %d failed", successful, failed)
	if pulled > 0 {
		fmt.Printf(", %d pulled", pulled)
	}
	if pushed > 0 {
		fmt.Printf(", %d pushed", pushed)
	}
	if conflicts > 0 {
		fmt.Printf(", %d conflicts", conflicts)
	}
	fmt.Println()

	// Detailed results table
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "REPOSITORY\tSTATUS\tOPERATIONS\tAHEAD/BEHIND\tERROR\n")
	fmt.Fprintf(w, "----------\t------\t----------\t------------\t-----\n")

	for _, result := range results {
		status := "✅"
		if !result.Success {
			status = "❌"
		} else if result.Conflicts {
			status = "⚠️"
		}

		operations := ""
		if result.Pulled {
			operations += "pulled "
		}
		if result.Pushed {
			operations += "pushed "
		}
		if operations == "" {
			operations = "-"
		}

		aheadBehind := fmt.Sprintf("%d/%d", result.AheadAfter, result.BehindAfter)
		if result.AheadAfter == 0 && result.BehindAfter == 0 {
			aheadBehind = "-"
		}

		errorMsg := ""
		if result.Error != "" {
			errorMsg = result.Error
			if len(errorMsg) > 30 {
				errorMsg = errorMsg[:27] + "..."
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			result.Repository,
			status,
			operations,
			aheadBehind,
			errorMsg)
	}

	err := w.Flush()
	if err != nil {
		return err
	}

	// Show conflicts and errors in detail
	if conflicts > 0 {
		fmt.Println()
		output.PrintHeader("Repositories with Conflicts")
		for _, result := range results {
			if result.Conflicts {
				fmt.Printf("❌ %s: %s\n", result.Repository, result.Error)
			}
		}
		fmt.Println()
		output.PrintInfo("Resolve conflicts manually and then run sync again")
	}

	if failed > 0 {
		fmt.Println()
		output.PrintHeader("Failed Operations")
		for _, result := range results {
			if !result.Success && !result.Conflicts {
				fmt.Printf("❌ %s: %s\n", result.Repository, result.Error)
			}
		}
	}

	return nil
}
