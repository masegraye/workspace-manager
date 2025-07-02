package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewListCommandV2() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-v2",
		Short: "List repositories and workspaces (new architecture)",
		Long:  "List discovered repositories and created workspaces using the new service architecture.",
	}

	cmd.AddCommand(
		NewListReposCommandV2(),
		NewListWorkspacesCommandV2(),
	)

	return cmd
}

func NewListReposCommandV2() *cobra.Command {
	var (
		format string
		tags   []string
	)

	cmd := &cobra.Command{
		Use:   "repos",
		Short: "List discovered repositories",
		Long:  "List all discovered repositories with optional filtering by tags.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListReposV2(cmd.Context(), format, tags)
		},
	}

	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Filter by tags (comma-separated)")

	return cmd
}

func NewListWorkspacesCommandV2() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "List created workspaces",
		Long:  "List all created workspaces, sorted by creation date (newest first).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListWorkspacesV2(cmd.Context(), format)
		},
	}

	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")

	return cmd
}

func runListReposV2(ctx context.Context, format string, tags []string) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// Get repositories, optionally filtered by tags
	repos, err := workspaceService.ListRepositoriesByTags(tags)
	if err != nil {
		return errors.Wrap(err, "failed to list repositories")
	}

	if len(repos) == 0 {
		if len(tags) > 0 {
			deps.Logger.Info("No repositories found with specified tags", 
				ux.Field("tags", strings.Join(tags, ", ")))
		} else {
			deps.Logger.Info("No repositories found. Run 'wsm discover-v2' to scan for repositories")
		}
		return nil
	}

	switch format {
	case "table":
		return printReposTableV2(repos)
	case "json":
		return printReposJSONV2(repos)
	default:
		return errors.Errorf("unsupported format: %s", format)
	}
}

func runListWorkspacesV2(ctx context.Context, format string) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	workspaces, err := workspaceService.ListWorkspaces()
	if err != nil {
		return errors.Wrap(err, "failed to list workspaces")
	}

	if len(workspaces) == 0 {
		deps.Logger.Info("No workspaces found. Use 'wsm create-v2' to create a workspace")
		return nil
	}

	// Sort workspaces by creation date descending (newest first)
	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].Created.After(workspaces[j].Created)
	})

	switch format {
	case "table":
		return printWorkspacesTableV2(workspaces)
	case "json":
		return printWorkspacesJSONV2(workspaces)
	default:
		return errors.Errorf("unsupported format: %s", format)
	}
}

func printReposTableV2(repos []domain.Repository) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			fmt.Printf("Failed to flush table writer: %v\n", err)
		}
	}()

	fmt.Fprintln(w, "NAME\tPATH\tBRANCH\tTAGS\tREMOTE")
	fmt.Fprintln(w, "----\t----\t------\t----\t------")

	for _, repo := range repos {
		tags := strings.Join(repo.Categories, ",")
		if len(tags) > 30 {
			tags = tags[:27] + "..."
		}

		remote := repo.RemoteURL
		if len(remote) > 50 {
			remote = "..." + remote[len(remote)-47:]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			repo.Name,
			repo.Path,
			repo.CurrentBranch,
			tags,
			remote,
		)
	}

	return nil
}

func printReposJSONV2(repos []domain.Repository) error {
	data, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal repositories to JSON")
	}
	fmt.Println(string(data))
	return nil
}

func printWorkspacesTableV2(workspaces []domain.Workspace) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			fmt.Printf("Failed to flush table writer: %v\n", err)
		}
	}()

	fmt.Fprintln(w, "NAME\tPATH\tREPOS\tBRANCH\tCREATED")
	fmt.Fprintln(w, "----\t----\t-----\t------\t-------")

	for _, workspace := range workspaces {
		repoNames := make([]string, len(workspace.Repositories))
		for i, repo := range workspace.Repositories {
			repoNames[i] = repo.Name
		}
		repos := strings.Join(repoNames, ",")
		if len(repos) > 30 {
			repos = repos[:27] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			workspace.Name,
			workspace.Path,
			repos,
			workspace.Branch,
			workspace.Created.Format("2006-01-02 15:04"),
		)
	}

	return nil
}

func printWorkspacesJSONV2(workspaces []domain.Workspace) error {
	data, err := json.MarshalIndent(workspaces, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal workspaces to JSON")
	}
	fmt.Println(string(data))
	return nil
}
