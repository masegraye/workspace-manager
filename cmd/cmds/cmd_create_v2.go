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
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// NewCreateCommandV2 creates the new service-based create command
func NewCreateCommandV2() *cobra.Command {
	var (
		repos        []string
		branch       string
		branchPrefix string
		baseBranch   string
		agentSource  string
		interactive  bool
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:   "create-v2 [workspace-name]",
		Short: "Create a new multi-repository workspace (new architecture)",
		Long: `Create a new workspace with specified repositories using the new service architecture.
The workspace will contain git worktrees for each repository on the specified branch.

If no branch is specified, a branch will be automatically created using the pattern:
  <branch-prefix>/<workspace-name>

Examples:
  # Create workspace with automatic branch (task/my-feature)
  wsm create-v2 my-feature --repos app,lib

  # Create workspace with custom branch
  wsm create-v2 my-feature --repos app,lib --branch feature/new-api

  # Create workspace with custom branch prefix (bug/my-feature)
  wsm create-v2 my-feature --repos app,lib --branch-prefix bug

  # Create workspace from specific base branch
  wsm create-v2 my-feature --repos app,lib --base-branch main

  # Interactive mode
  wsm create-v2 my-feature --interactive`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateV2(cmd.Context(), args[0], repos, branch, branchPrefix, baseBranch, agentSource, interactive, dryRun)
		},
	}

	cmd.Flags().StringSliceVar(&repos, "repos", nil, "Repository names to include (comma-separated)")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name for worktrees (if not specified, uses <branch-prefix>/<workspace-name>)")
	cmd.Flags().StringVar(&branchPrefix, "branch-prefix", "task", "Prefix for auto-generated branch names")
	cmd.Flags().StringVar(&baseBranch, "base-branch", "", "Base branch to create new branch from (defaults to current branch)")
	cmd.Flags().StringVar(&agentSource, "agent-source", "", "Path to AGENT.md template file")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Interactive repository selection")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be created without actually creating")

	return cmd
}

func runCreateV2(ctx context.Context, name string, repos []string, branch, branchPrefix, baseBranch, agentSource string, interactive, dryRun bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Handle interactive mode
	if interactive {
		selectedRepos, err := selectRepositoriesInteractivelyV2(workspaceService, deps.Prompter)
		if err != nil {
			// Check if user cancelled - handle gracefully without error
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "cancelled") || strings.Contains(errMsg, "interrupt") {
				deps.Logger.Info("Operation cancelled by user")
				return nil
			}
			return errors.Wrap(err, "interactive selection failed")
		}
		repos = selectedRepos
	}

	// Validate inputs
	if len(repos) == 0 {
		return errors.New("no repositories specified. Use --repos flag or --interactive mode")
	}

	// Generate branch name if not specified
	finalBranch := branch
	if finalBranch == "" {
		finalBranch = fmt.Sprintf("%s/%s", branchPrefix, name)
		deps.Logger.Info("Using auto-generated branch", ux.Field("branch", finalBranch))
		log.Debug().Str("branch", finalBranch).Str("prefix", branchPrefix).Str("name", name).Msg("Generated branch name")
	}

	// Load AGENT.md content if specified
	var agentMDContent string
	if agentSource != "" {
		content, err := deps.FS.ReadFile(agentSource)
		if err != nil {
			return errors.Wrapf(err, "failed to read agent source file: %s", agentSource)
		}
		agentMDContent = string(content)
	}

	// Create workspace using the new service
	deps.Logger.Info("Creating workspace",
		ux.Field("name", name),
		ux.Field("repos", repos),
		ux.Field("branch", finalBranch),
		ux.Field("baseBranch", baseBranch),
		ux.Field("dryRun", dryRun))

	workspace, err := workspaceService.Create(ctx, service.CreateRequest{
		Name:       name,
		RepoNames:  repos,
		Branch:     finalBranch,
		BaseBranch: baseBranch,
		AgentMD:    agentMDContent,
		DryRun:     dryRun,
	})

	if err != nil {
		// Check if user cancelled - handle gracefully without error
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "cancelled") || strings.Contains(errMsg, "interrupt") {
			deps.Logger.Info("Operation cancelled by user")
			return nil
		}
		return errors.Wrap(err, "failed to create workspace")
	}

	// Show results
	if dryRun {
		return showWorkspacePreviewV2(*workspace, deps.Logger)
	}

	// Success output
	output.PrintSuccess("Workspace '%s' created successfully!", workspace.Name)
	fmt.Println()

	output.PrintHeader("Workspace Details")
	fmt.Printf("  Path: %s\n", workspace.Path)

	repoNames := make([]string, len(workspace.Repositories))
	for i, repo := range workspace.Repositories {
		repoNames[i] = repo.Name
	}
	fmt.Printf("  Repositories: %s\n", strings.Join(repoNames, ", "))

	if workspace.Branch != "" {
		fmt.Printf("  Branch: %s\n", workspace.Branch)
	}
	if workspace.GoWorkspace {
		fmt.Printf("  Go workspace: yes (go.work created)\n")
	}
	if workspace.AgentMD != "" {
		fmt.Printf("  AGENT.md: created\n")
	}

	fmt.Println()
	output.PrintInfo("To start working:")
	fmt.Printf("  cd %s\n", workspace.Path)

	deps.Logger.Info("Workspace creation completed successfully",
		ux.Field("name", workspace.Name),
		ux.Field("path", workspace.Path))

	return nil
}

func selectRepositoriesInteractivelyV2(workspaceService *service.WorkspaceService, prompter ux.Prompter) ([]string, error) {
	// Get available repositories from the registry
	repos, err := workspaceService.ListRepositories()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load repositories")
	}

	if len(repos) == 0 {
		return nil, errors.New("no repositories found. Run 'wsm discover' first to find repositories")
	}

	// Convert to options for the prompter
	options := make([]string, len(repos))
	for i, repo := range repos {
		// Show name and categories for better selection
		categories := ""
		if len(repo.Categories) > 0 {
			categories = fmt.Sprintf(" [%s]", strings.Join(repo.Categories, ", "))
		}
		options[i] = fmt.Sprintf("%s%s", repo.Name, categories)
	}

	// Use multi-select if prompter supports it
	if multiPrompter, ok := prompter.(ux.MultiSelectPrompter); ok {
		selected, err := multiPrompter.MultiSelect("Select repositories for the workspace:", options)
		if err != nil {
			return nil, err
		}

		// Extract repo names from the selected options
		var repoNames []string
		for _, selection := range selected {
			// Extract the name part before any categories
			name := strings.Split(selection, " [")[0]
			repoNames = append(repoNames, name)
		}

		return repoNames, nil
	}

	// Fallback to single selection if multi-select not supported
	selected, err := prompter.Select("Select a repository for the workspace:", options)
	if err != nil {
		return nil, err
	}

	// Extract repo name from the selected option
	name := strings.Split(selected, " [")[0]
	return []string{name}, nil
}

func showWorkspacePreviewV2(workspace domain.Workspace, logger ux.Logger) error {
	output.PrintHeader("Dry Run - Workspace Preview")
	fmt.Printf("  Name: %s\n", workspace.Name)
	fmt.Printf("  Path: %s\n", workspace.Path)

	repoNames := make([]string, len(workspace.Repositories))
	for i, repo := range workspace.Repositories {
		repoNames[i] = repo.Name
	}
	fmt.Printf("  Repositories: %s\n", strings.Join(repoNames, ", "))

	if workspace.Branch != "" {
		fmt.Printf("  Branch: %s\n", workspace.Branch)
	}
	if workspace.GoWorkspace {
		fmt.Printf("  Go workspace: yes (go.work would be created)\n")
	}
	if workspace.AgentMD != "" {
		fmt.Printf("  AGENT.md: would be created\n")
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
