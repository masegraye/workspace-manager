package cmds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewDiscoverCommandV2() *cobra.Command {
	var (
		recursive bool
		maxDepth  int
		verbose   bool
	)

	cmd := &cobra.Command{
		Use:   "discover-v2 [paths...]",
		Short: "Discover git repositories using the new service architecture",
		Long: `Discover git repositories in the specified directories and add them to the registry.
If no paths are specified, defaults to current directory.

The new architecture provides:
- Automatic project type detection (Go, Node.js, Python, Rust, Java, Docker, Web)
- Better error handling and rollback capabilities
- Structured logging throughout the discovery process
- Comprehensive testing support

Examples:
  # Discover repositories in current directory
  wsm discover-v2

  # Discover in specific paths with maximum depth
  wsm discover-v2 ~/code ~/projects --max-depth 2

  # Non-recursive discovery
  wsm discover-v2 ~/code --recursive=false

  # Verbose output showing categories detected
  wsm discover-v2 ~/code --verbose`,
		Args: cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverV2(cmd.Context(), args, recursive, maxDepth, verbose)
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "Recursively scan subdirectories")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 3, "Maximum depth for recursive scanning")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output showing detected categories")

	return cmd
}

func runDiscoverV2(ctx context.Context, paths []string, recursive bool, maxDepth int, verbose bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Default to current directory if no paths specified
	if len(paths) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}
		paths = []string{cwd}
	}

	// Expand and validate paths
	expandedPaths, err := expandAndValidatePaths(paths, deps)
	if err != nil {
		return err
	}

	// Discover repositories using the new service
	deps.Logger.Info("Starting repository discovery", 
		ux.Field("paths", expandedPaths),
		ux.Field("recursive", recursive),
		ux.Field("maxDepth", maxDepth))

	output.PrintInfo("Discovering repositories in %v", expandedPaths)

	err = workspaceService.DiscoverRepositories(ctx, expandedPaths, recursive, maxDepth)
	if err != nil {
		return errors.Wrap(err, "discovery failed")
	}

	// Get and display results
	repos, err := workspaceService.ListRepositories()
	if err != nil {
		return errors.Wrap(err, "failed to retrieve discovered repositories")
	}

	output.PrintSuccess("Discovery complete! Found %d repositories", len(repos))

	if len(repos) > 0 {
		if verbose {
			displayRepositoriesVerbose(repos)
		} else {
			displayRepositoriesSummary(repos)
		}
		
		fmt.Println()
		output.PrintInfo("Use 'wsm list repos' to see all repositories, or 'wsm create-v2 <name> --interactive' to create workspaces")
	}

	deps.Logger.Info("Repository discovery completed", 
		ux.Field("totalRepositories", len(repos)))

	return nil
}

func expandAndValidatePaths(paths []string, deps *service.Deps) ([]string, error) {
	var expandedPaths []string
	
	for _, path := range paths {
		// Expand ~ to home directory
		if len(path) > 0 && path[0] == '~' {
			home, err := deps.FS.UserHomeDir()
			if err != nil {
				return nil, errors.Wrap(err, "failed to get home directory")
			}
			if len(path) == 1 {
				path = home
			} else {
				path = deps.FS.Join(home, path[1:])
			}
		}

		// Convert to absolute path
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get absolute path for %s", path)
		}

		// Check if path exists
		if !deps.FS.Exists(absPath) {
			return nil, errors.Errorf("path does not exist: %s", absPath)
		}

		expandedPaths = append(expandedPaths, absPath)
	}
	
	return expandedPaths, nil
}

func displayRepositoriesSummary(repos []domain.Repository) {
	fmt.Println()
	output.PrintHeader("Discovered Repositories")
	
	// Group by categories for summary
	categoryCount := make(map[string]int)
	for _, repo := range repos {
		for _, category := range repo.Categories {
			categoryCount[category]++
		}
	}
	
	if len(categoryCount) > 0 {
		fmt.Printf("Project types found:\n")
		for category, count := range categoryCount {
			fmt.Printf("  %s: %d repositories\n", category, count)
		}
	}
	
	// Show first few repositories as examples
	fmt.Printf("\nRepositories (showing first 10):\n")
	for i, repo := range repos {
		if i >= 10 {
			fmt.Printf("  ... and %d more\n", len(repos)-10)
			break
		}
		fmt.Printf("  📁 %s (%s)\n", repo.Name, repo.Path)
	}
}

func displayRepositoriesVerbose(repos []domain.Repository) {
	fmt.Println()
	output.PrintHeader("Discovered Repositories (Detailed)")
	
	for _, repo := range repos {
		fmt.Printf("📁 %s\n", repo.Name)
		fmt.Printf("   Path: %s\n", repo.Path)
		
		if repo.RemoteURL != "" {
			fmt.Printf("   Remote: %s\n", repo.RemoteURL)
		}
		
		if repo.CurrentBranch != "" {
			fmt.Printf("   Branch: %s\n", repo.CurrentBranch)
		}
		
		if len(repo.Categories) > 0 {
			fmt.Printf("   Categories: %s\n", strings.Join(repo.Categories, ", "))
		}
		
		if len(repo.Branches) > 0 {
			branches := repo.Branches
			if len(branches) > 5 {
				branches = append(branches[:5], "...")
			}
			fmt.Printf("   Branches: %s\n", strings.Join(branches, ", "))
		}
		
		if len(repo.Tags) > 0 {
			tags := repo.Tags
			if len(tags) > 3 {
				tags = append(tags[:3], "...")
			}
			fmt.Printf("   Tags: %s\n", strings.Join(tags, ", "))
		}
		
		fmt.Println()
	}
}
