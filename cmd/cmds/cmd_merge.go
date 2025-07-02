package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/carapace-sh/carapace"
	"github.com/charmbracelet/huh"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewMergeCommand() *cobra.Command {
	var (
		dryRun        bool
		force         bool
		workspace     string
		keepWorkspace bool
		jsonOut       bool
	)

	cmd := &cobra.Command{
		Use:   "merge [workspace-name]",
		Short: "Merge a forked workspace back into its base branch using the new service architecture",
		Long: `Merge a forked workspace back into its base branch and optionally delete the workspace using the new service architecture.

The new architecture provides:
- Parallel repository processing for faster operations
- Better error handling and rollback capabilities
- Structured output with JSON support
- Enhanced validation and conflict detection
- Improved merge location validation

This command:
1. Detects the current workspace (if not specified)
2. Verifies the workspace is a fork (has a base branch)
3. Checks if a workspace exists for the base branch and enforces running from within it
4. Validates that all repositories are clean before merging
5. For each repository:
   - Switches to the base branch
   - Merges the workspace branch into the base branch
   - Pushes the merged changes
6. Optionally deletes the workspace after successful merge

The command handles merge conflicts gracefully and provides rollback on failure.

IMPORTANT: If there's an existing workspace for the base branch, you must run this
command from within that workspace to avoid git worktree conflicts. The command
will detect this situation and provide guidance if you're in the wrong location.

Examples:
  # Merge current workspace back to its base branch
  wsm merge

  # Merge a specific workspace
  wsm merge my-feature-workspace

  # Preview what would be merged without executing
  wsm merge --dry-run

  # Merge without asking for confirmation
  wsm merge --force

  # Merge but keep the workspace (don't delete)
  wsm merge --keep-workspace

  # JSON output for scripting
  wsm merge --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := workspace
			if len(args) > 0 {
				workspaceName = args[0]
			}
			return runMergeV2(cmd.Context(), workspaceName, service.MergeRequest{
				WorkspaceName: workspaceName,
				DryRun:        dryRun,
				Force:         force,
				KeepWorkspace: keepWorkspace,
			}, jsonOut)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be merged without executing")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompts and force merge even with uncommitted changes")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name")
	cmd.Flags().BoolVar(&keepWorkspace, "keep-workspace", false, "Keep the workspace after merge (don't delete it)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output result as JSON")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runMergeV2(ctx context.Context, workspaceName string, req service.MergeRequest, jsonOut bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// If no workspace specified, try to detect current workspace
	if workspaceName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}

		detectedName, err := workspaceService.DetectWorkspace(cwd)
		if err != nil {
			return errors.Wrap(err, "failed to detect workspace. Use 'wsm merge <workspace-name>' or specify --workspace flag")
		}
		workspaceName = detectedName
		req.WorkspaceName = workspaceName
	}

	deps.Logger.Info("Starting merge operation",
		ux.Field("workspace", workspaceName),
		ux.Field("dryRun", req.DryRun),
		ux.Field("force", req.Force),
		ux.Field("keepWorkspace", req.KeepWorkspace))

	// Ask for confirmation unless force is set or dry-run
	if !req.DryRun && !req.Force && !jsonOut {
		confirmed, err := confirmMergeV2(workspaceName, req.KeepWorkspace)
		if err != nil {
			return errors.Wrap(err, "failed to get user confirmation")
		}
		if !confirmed {
			deps.Logger.Info("Merge cancelled by user")
			return nil
		}
	}

	// Merge workspace using the new service
	response, err := workspaceService.MergeWorkspace(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed to merge workspace")
	}

	// Display results based on format
	if jsonOut {
		return printMergeResultJSON(response)
	}

	return printMergeResultDetailed(response, req.DryRun)
}

func confirmMergeV2(workspaceName string, keepWorkspace bool) (bool, error) {
	fmt.Printf("\n")
	output.PrintWarning("You are about to merge workspace '%s'", workspaceName)

	if !keepWorkspace {
		fmt.Printf("  The workspace will be DELETED after successful merge\n")
	}
	fmt.Println()

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Do you want to proceed with the merge?").
				Value(&confirmed),
		),
	)

	err := form.Run()
	if err != nil {
		return false, err
	}

	return confirmed, nil
}

func printMergeResultJSON(response *service.MergeResponse) error {
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal merge response to JSON")
	}
	fmt.Println(string(data))
	return nil
}

func printMergeResultDetailed(response *service.MergeResponse, dryRun bool) error {
	if dryRun {
		output.PrintHeader("🔀 Merge Preview: %s", response.WorkspaceName)
		fmt.Println()

		fmt.Printf("Workspace: %s\n", response.WorkspaceName)
		fmt.Printf("Branch: %s → %s\n", response.CurrentBranch, response.BaseBranch)
		fmt.Printf("Repositories: %d\n", len(response.Candidates))
		fmt.Println()

		output.PrintInfo("Merge Plan:")
		for _, candidate := range response.Candidates {
			status := "✓ Clean"
			if !candidate.IsClean {
				status = "⚠️  Has changes"
			}

			fmt.Printf("  %s (%s)\n", candidate.Repository.Name, status)
			fmt.Printf("    Merge: %s → %s\n", response.CurrentBranch, response.BaseBranch)
			fmt.Printf("    Push: %s to origin\n", response.BaseBranch)
		}

		fmt.Println()
		output.PrintInfo("After successful merge:")
		fmt.Printf("  - All repositories will have %s branch updated\n", response.BaseBranch)
		fmt.Printf("  - Changes will be pushed to origin\n")
		if !response.WorkspaceDeleted {
			fmt.Printf("  - Workspace will be deleted\n")
		}
	} else {
		output.PrintHeader("🔀 Merge Results: %s", response.WorkspaceName)
		fmt.Println()

		fmt.Printf("Branch: %s → %s\n", response.CurrentBranch, response.BaseBranch)
		fmt.Printf("Repositories processed: %d\n", len(response.Candidates))
		fmt.Printf("Successful merges: %d\n", len(response.MergedRepos))

		if len(response.Errors) > 0 {
			fmt.Printf("Errors: %d\n", len(response.Errors))
		}

		fmt.Println()

		// Show successful merges
		if len(response.MergedRepos) > 0 {
			fmt.Println("Successfully merged:")
			for _, repo := range response.MergedRepos {
				fmt.Printf("✅ %s\n", repo)
			}
			fmt.Println()
		}

		// Show errors
		if len(response.Errors) > 0 {
			fmt.Println("Errors:")
			for repo, errMsg := range response.Errors {
				fmt.Printf("❌ %s: %s\n", repo, errMsg)
			}
			fmt.Println()
		}

		// Show summary
		if len(response.MergedRepos) == 0 && len(response.Errors) == 0 {
			output.PrintInfo("No repositories found to merge")
		} else if len(response.MergedRepos) > 0 && len(response.Errors) == 0 {
			output.PrintSuccess("Successfully merged workspace '%s'", response.WorkspaceName)
			fmt.Printf("  - Merged %d repositories\n", len(response.MergedRepos))
			fmt.Printf("  - Branch %s merged into %s\n", response.CurrentBranch, response.BaseBranch)
			fmt.Printf("  - Changes pushed to origin\n")
			if response.WorkspaceDeleted {
				fmt.Printf("  - Workspace deleted\n")
			}
		}
	}

	return nil
}
