package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewRebaseCommand() *cobra.Command {
	var (
		targetBranch string
		repository   string
		dryRun       bool
		interactive  bool
		workspace    string
		jsonOut      bool
		verbose      bool
	)

	cmd := &cobra.Command{
		Use:   "rebase [repository]",
		Short: "Rebase workspace repositories using the new service architecture",
		Long: `Rebase workspace repositories against a target branch using the new service architecture.

The new architecture provides:
- Faster rebase operations with parallel repository processing
- Better error handling and conflict detection
- Structured output with JSON support
- Clean separation between rebase logic and presentation
- Detailed tracking of commits before/after rebase

By default, rebases all repositories in the workspace against the 'main' branch.
You can specify a specific repository to rebase or change the target branch.

Examples:
  # Rebase all repositories against main
  wsm rebase

  # Rebase specific repository against main  
  wsm rebase my-repo

  # Rebase all repositories against develop
  wsm rebase --target develop

  # Rebase specific repository against feature/base
  wsm rebase my-repo --target feature/base

  # Interactive rebase
  wsm rebase my-repo --interactive

  # Dry run to see what would be done
  wsm rebase --dry-run

  # JSON output for scripting
  wsm rebase --json

  # Verbose output with detailed information
  wsm rebase --verbose`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				repository = args[0]
			}
			return runRebaseV2(cmd.Context(), workspace, repository, targetBranch, interactive, dryRun, jsonOut, verbose)
		},
	}

	cmd.Flags().StringVar(&targetBranch, "target", "main", "Target branch to rebase onto")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without actually rebasing")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive rebase")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output results as JSON")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed rebase information")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runRebaseV2(ctx context.Context, workspacePath, repository, targetBranch string, interactive, dryRun, jsonOut, verbose bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// If no workspace specified, try to detect current workspace
	if workspacePath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}
		workspacePath = cwd
	}

	// Load workspace from path
	workspace, err := workspaceService.LoadWorkspaceFromPath(workspacePath)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace from '%s'", workspacePath)
	}

	if repository != "" {
		deps.Logger.Info("Rebasing repository against target branch",
			ux.Field("workspace", workspace.Name),
			ux.Field("repository", repository),
			ux.Field("target_branch", targetBranch))
	} else {
		deps.Logger.Info("Rebasing all repositories against target branch",
			ux.Field("workspace", workspace.Name),
			ux.Field("target_branch", targetBranch))
	}

	if dryRun {
		deps.Logger.Info("Dry run mode - no changes will be made")
	}

	// Create rebase request
	request := service.RebaseRequest{
		TargetBranch: targetBranch,
		Repository:   repository,
		Interactive:  interactive,
		DryRun:       dryRun,
	}

	// Perform rebase using the service
	response, err := workspaceService.RebaseWorkspace(ctx, *workspace, request)
	if err != nil {
		return errors.Wrap(err, "failed to rebase workspace")
	}

	// Display results based on format
	if jsonOut {
		return printRebaseResultsJSON(response)
	}

	return printRebaseResultsV2(response, dryRun, verbose)
}

func printRebaseResultsJSON(response *service.RebaseResponse) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(response)
}

func printRebaseResultsV2(response *service.RebaseResponse, dryRun, verbose bool) error {
	if len(response.Results) == 0 {
		output.PrintInfo("No repositories to rebase.")
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

	// Header
	if verbose {
		fmt.Fprintln(w, "\nREPOSITORY\tSTATUS\tCURRENT\tTARGET\tCOMMITS BEFORE\tCOMMITS AFTER\tERROR")
		fmt.Fprintln(w, "----------\t------\t-------\t------\t--------------\t-------------\t-----")
	} else {
		fmt.Fprintln(w, "\nREPOSITORY\tSTATUS\tTARGET\tCOMMITS BEFORE\tCOMMITS AFTER\tERROR")
		fmt.Fprintln(w, "----------\t------\t------\t--------------\t-------------\t-----")
	}

	// Results
	for _, result := range response.Results {
		status := "✅"
		if !result.Success {
			status = "❌"
		}

		if result.Conflicts {
			status = "⚠️"
		}

		commitsBefore := "-"
		if result.CommitsBefore > 0 {
			commitsBefore = fmt.Sprintf("%d", result.CommitsBefore)
		}

		commitsAfter := "-"
		if result.CommitsAfter > 0 {
			commitsAfter = fmt.Sprintf("%d", result.CommitsAfter)
		}

		errorMsg := result.Error
		if len(errorMsg) > 30 {
			errorMsg = errorMsg[:27] + "..."
		}

		if verbose {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				result.Repository,
				status,
				result.CurrentBranch,
				result.TargetBranch,
				commitsBefore,
				commitsAfter,
				errorMsg,
			)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				result.Repository,
				status,
				result.TargetBranch,
				commitsBefore,
				commitsAfter,
				errorMsg,
			)
		}
	}

	fmt.Fprintln(w)

	// Summary
	totalRepos := len(response.Results)
	output.PrintSuccess("Summary: %d/%d repositories rebased successfully", response.SuccessCount, totalRepos)

	if response.ErrorCount > 0 {
		output.PrintError("%d repositories failed to rebase", response.ErrorCount)
	}

	if response.ConflictCount > 0 {
		output.PrintWarning("%d repositories have conflicts", response.ConflictCount)
		output.PrintInfo("Resolve conflicts manually with:")
		fmt.Println("  - Fix conflicts in the affected files")
		fmt.Println("  - git add <resolved-files>")
		fmt.Println("  - git rebase --continue")
		fmt.Println("  Or abort the rebase with: git rebase --abort")
	}

	return nil
}
