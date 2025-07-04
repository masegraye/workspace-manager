package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
)

func NewCommitCommand() *cobra.Command {
	var (
		message      string
		interactive  bool
		addAll       bool
		push         bool
		dryRun       bool
		template     string
		workspace    string
		jsonOut      bool
		repositories []string
	)

	cmd := &cobra.Command{
		Use:   "commit [workspace-path]",
		Short: "Commit changes across workspace repositories using the new service architecture",
		Long: `Commit related changes across multiple repositories in the workspace using the new service architecture.

The new architecture provides:
- Parallel repository processing for faster operations
- Better error handling and recovery
- Structured output with JSON support
- Improved commit message templating
- Enhanced repository filtering capabilities

Features:
- Interactive file selection (planned)
- Consistent commit messaging across repositories
- Batch operations with rollback support
- Support for conventional commit templates
- Selective repository commits

If no workspace path is provided, attempts to detect the current workspace from the working directory.

Examples:
  # Commit changes in current workspace
  wsm commit -m "feat: add new feature"

  # Commit changes in specific workspace
  wsm commit /path/to/workspace -m "fix: resolve issue"

  # Use commit message template
  wsm commit --template feature -m "implement user authentication"

  # Add all files and commit
  wsm commit -m "chore: update dependencies" --add-all

  # Commit and push changes
  wsm commit -m "docs: update README" --push

  # Dry run to see what would be committed
  wsm commit -m "test commit" --dry-run

  # Interactive mode for file selection
  wsm commit --interactive

  # Commit only specific repositories
  wsm commit -m "fix: auth service" --repositories auth,shared

  # JSON output for scripting
  wsm commit -m "feat: new endpoint" --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runCommitV2(cmd.Context(), workspacePath, service.CommitRequest{
				Message:      message,
				Interactive:  interactive,
				AddAll:       addAll,
				Push:         push,
				DryRun:       dryRun,
				Template:     template,
				Repositories: repositories,
			}, jsonOut)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Interactive file selection")
	cmd.Flags().BoolVar(&addAll, "add-all", false, "Add all changes before committing")
	cmd.Flags().BoolVar(&push, "push", false, "Push changes after commit")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be committed without making changes")
	cmd.Flags().StringVar(&template, "template", "", "Use commit message template (feature, fix, docs, style, refactor, test, chore)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output result as JSON")
	cmd.Flags().StringSliceVar(&repositories, "repositories", []string{}, "Specific repositories to commit (comma-separated)")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runCommitV2(ctx context.Context, workspacePath string, req service.CommitRequest, jsonOut bool) error {
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

	deps.Logger.Info("Starting commit operation",
		ux.Field("workspace", workspace.Name),
		ux.Field("path", workspace.Path),
		ux.Field("message", req.Message),
		ux.Field("dry_run", req.DryRun))

	// Commit changes using the new service
	response, err := workspaceService.CommitWorkspace(ctx, *workspace, req)
	if err != nil {
		return errors.Wrap(err, "failed to commit workspace changes")
	}

	// Display results based on format
	if jsonOut {
		return printCommitResultJSON(response)
	}

	return printCommitResultDetailed(response, req.DryRun)
}

func printCommitResultJSON(response *service.CommitResponse) error {
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal commit response to JSON")
	}
	fmt.Println(string(data))
	return nil
}

func printCommitResultDetailed(response *service.CommitResponse, dryRun bool) error {
	if dryRun {
		output.PrintHeader("Commit Preview (Dry Run)")
	} else {
		output.PrintHeader("Commit Results")
	}

	fmt.Printf("Message: %s\n", response.Message)
	fmt.Printf("Repositories processed: %d\n", len(response.CommittedRepos)+len(response.Errors))
	fmt.Printf("Successful commits: %d\n", len(response.CommittedRepos))

	if len(response.Errors) > 0 {
		fmt.Printf("Errors: %d\n", len(response.Errors))
	}

	fmt.Println()

	// Show successful commits
	if len(response.CommittedRepos) > 0 {
		if dryRun {
			fmt.Println("Would be committed:")
		} else {
			fmt.Println("Successfully committed:")
		}

		for _, repo := range response.CommittedRepos {
			changes := response.Changes[repo]
			fmt.Printf("✅ %s (%d files)\n", repo, len(changes))

			if len(changes) > 0 {
				// Show first few files
				displayFiles := changes
				if len(displayFiles) > 5 {
					displayFiles = changes[:5]
				}

				for _, file := range displayFiles {
					fmt.Printf("   • %s\n", file)
				}

				if len(changes) > 5 {
					fmt.Printf("   ... and %d more files\n", len(changes)-5)
				}
			}
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
	if len(response.CommittedRepos) == 0 && len(response.Errors) == 0 {
		output.PrintInfo("No changes found in workspace")
	} else if !dryRun && len(response.CommittedRepos) > 0 {
		output.PrintSuccess("Successfully committed changes across %d repositories", len(response.CommittedRepos))
	}

	return nil
}
