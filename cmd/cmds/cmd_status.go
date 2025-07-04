package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewStatusCommand() *cobra.Command {
	var (
		short     bool
		untracked bool
		workspace string
		jsonOut   bool
		verbose   bool
	)

	cmd := &cobra.Command{
		Use:   "status [workspace-path]",
		Short: "Show workspace status",
		Long: `Show the git status of all repositories in a workspace.

If no workspace path is provided, attempts to detect the current workspace from the working directory.

Examples:
  # Show status of current workspace
  wsm status

  # Show status of specific workspace
  wsm status /path/to/workspace

  # Show compact status format
  wsm status --short

  # Include untracked files in output
  wsm status --untracked

  # JSON output for scripting
  wsm status --json

  # Verbose output with detailed information
  wsm status --verbose`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspacePath := workspace
			if len(args) > 0 {
				workspacePath = args[0]
			}
			return runStatusV2(cmd.Context(), workspacePath, short, untracked, jsonOut, verbose)
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "Show short status format")
	cmd.Flags().BoolVar(&untracked, "untracked", false, "Include untracked files")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace path")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output status as JSON")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed repository information")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runStatusV2(ctx context.Context, workspacePath string, short, untracked, jsonOut, verbose bool) error {
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

	deps.Logger.Info("Getting workspace status",
		ux.Field("workspace", workspace.Name),
		ux.Field("path", workspace.Path))

	// Get status using the new service
	status, err := workspaceService.GetWorkspaceStatus(ctx, *workspace)
	if err != nil {
		return errors.Wrap(err, "failed to get workspace status")
	}

	// Display status based on format
	if jsonOut {
		return printStatusJSON(status)
	}

	if short {
		return printStatusShortV2(status, untracked)
	}

	return printStatusDetailedV2(status, untracked, verbose)
}

func printStatusJSON(status *domain.WorkspaceStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal status to JSON")
	}
	fmt.Println(string(data))
	return nil
}

func printStatusShortV2(status *domain.WorkspaceStatus, includeUntracked bool) error {
	output.PrintHeader("Workspace Status: %s", status.Workspace.Name)
	fmt.Printf("Overall: %s\n", getStatusIcon(status.Overall))
	fmt.Printf("Path: %s\n", status.Workspace.Path)
	fmt.Printf("Repositories: %d\n\n", len(status.Repositories))

	// Create table writer for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "REPOSITORY\tSTATUS\tBRANCH\tAHEAD/BEHIND\n")
	fmt.Fprintf(w, "----------\t------\t------\t------------\n")

	for _, repo := range status.Repositories {
		statusIcon := getRepositoryStatusIcon(repo)
		aheadBehind := fmt.Sprintf("%d/%d", repo.Ahead, repo.Behind)
		if repo.Ahead == 0 && repo.Behind == 0 {
			aheadBehind = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			repo.Repository.Name,
			statusIcon,
			repo.CurrentBranch,
			aheadBehind)
	}

	return w.Flush()
}

func printStatusDetailedV2(status *domain.WorkspaceStatus, includeUntracked, verbose bool) error {
	output.PrintHeader("Workspace Status: %s", status.Workspace.Name)
	fmt.Printf("Overall: %s %s\n", getStatusIcon(status.Overall), status.Overall)
	fmt.Printf("Path: %s\n", status.Workspace.Path)
	fmt.Printf("Branch: %s\n", status.Workspace.Branch)
	if status.Workspace.BaseBranch != "" {
		fmt.Printf("Base Branch: %s\n", status.Workspace.BaseBranch)
	}
	fmt.Printf("Repositories: %d\n", len(status.Repositories))
	fmt.Println()

	// Show repository details
	for i, repo := range status.Repositories {
		if i > 0 {
			fmt.Println()
		}

		fmt.Printf("📁 %s\n", repo.Repository.Name)
		fmt.Printf("   Status: %s\n", getRepositoryStatusDescription(repo))
		fmt.Printf("   Branch: %s\n", repo.CurrentBranch)

		if repo.Ahead > 0 || repo.Behind > 0 {
			fmt.Printf("   Sync: %d ahead, %d behind\n", repo.Ahead, repo.Behind)
		}

		if repo.HasConflicts {
			fmt.Printf("   ⚠️  Has merge conflicts\n")
		}

		if repo.IsMerged {
			fmt.Printf("   ✅ Branch is merged\n")
		}

		if repo.NeedsRebase {
			fmt.Printf("   🔄 Needs rebase\n")
		}

		// Show file changes if verbose or if there are changes
		if verbose || repo.HasChanges {
			if len(repo.StagedFiles) > 0 {
				fmt.Printf("   Staged: %s\n", strings.Join(repo.StagedFiles, ", "))
			}
			if len(repo.ModifiedFiles) > 0 {
				fmt.Printf("   Modified: %s\n", strings.Join(repo.ModifiedFiles, ", "))
			}
			if includeUntracked && len(repo.UntrackedFiles) > 0 {
				untracked := repo.UntrackedFiles
				if len(untracked) > 5 {
					untracked = append(untracked[:5], "...")
				}
				fmt.Printf("   Untracked: %s\n", strings.Join(untracked, ", "))
			}
		}

		if verbose {
			if len(repo.Repository.Categories) > 0 {
				fmt.Printf("   Categories: %s\n", strings.Join(repo.Repository.Categories, ", "))
			}
			if repo.Repository.RemoteURL != "" {
				fmt.Printf("   Remote: %s\n", repo.Repository.RemoteURL)
			}
		}
	}

	return nil
}

func getStatusIcon(status string) string {
	switch status {
	case "clean":
		return "✅"
	case "dirty":
		return "📝"
	case "staged":
		return "📦"
	case "conflicts":
		return "❌"
	case "ahead":
		return "⬆️"
	case "behind":
		return "⬇️"
	case "diverged":
		return "🔀"
	default:
		return "❓"
	}
}

func getRepositoryStatusIcon(repo domain.RepositoryStatus) string {
	if repo.HasConflicts {
		return "❌"
	}
	if len(repo.StagedFiles) > 0 {
		return "📦"
	}
	if repo.HasChanges {
		return "📝"
	}
	if repo.Ahead > 0 && repo.Behind > 0 {
		return "🔀"
	}
	if repo.Ahead > 0 {
		return "⬆️"
	}
	if repo.Behind > 0 {
		return "⬇️"
	}
	return "✅"
}

func getRepositoryStatusDescription(repo domain.RepositoryStatus) string {
	if repo.HasConflicts {
		return "conflicts"
	}
	if len(repo.StagedFiles) > 0 {
		return "staged changes"
	}
	if repo.HasChanges {
		return "uncommitted changes"
	}
	if repo.Ahead > 0 && repo.Behind > 0 {
		return "diverged"
	}
	if repo.Ahead > 0 {
		return "ahead"
	}
	if repo.Behind > 0 {
		return "behind"
	}
	return "clean"
}
