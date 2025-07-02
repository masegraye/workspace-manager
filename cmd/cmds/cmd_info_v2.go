package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewInfoCommandV2() *cobra.Command {
	var (
		outputFormat string
		outputField  string
		workspace    string
	)

	cmd := &cobra.Command{
		Use:   "info-v2 [workspace-name]",
		Short: "Display workspace information (new architecture)",
		Long: `Display information about a workspace using the new service architecture.

By default, shows all workspace information. Use --field to get a specific piece of information.

Available fields:
  - path: workspace directory path
  - name: workspace name  
  - branch: workspace branch
  - repositories: number of repositories
  - created: creation date and time (YYYY-MM-DD HH:MM:SS)
  - date: creation date only (YYYY-MM-DD)
  - time: creation time only (HH:MM:SS)

Examples:
  # Show all workspace info
  wsm info-v2 my-workspace

  # Get just the path (useful for cd $(wsm info-v2 my-workspace --field path))
  wsm info-v2 my-workspace --field path

  # Get workspace name
  wsm info-v2 --field name

  # JSON output
  wsm info-v2 my-workspace --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := workspace
			if len(args) > 0 {
				workspaceName = args[0]
			}
			return runInfoV2(cmd.Context(), workspaceName, outputFormat, outputField)
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json)")
	cmd.Flags().StringVar(&outputField, "field", "", "Output specific field only (path, name, branch, repositories, created, date, time)")
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name")

	return cmd
}

func runInfoV2(ctx context.Context, workspaceName string, outputFormat, outputField string) error {
	// Initialize services
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	// If no workspace specified, try to detect current workspace
	if workspaceName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}

		detected, err := workspaceService.DetectWorkspace(cwd)
		if err != nil {
			return errors.Wrap(err, "failed to detect workspace. Use 'wsm info-v2 <workspace-name>' or specify --workspace flag")
		}
		workspaceName = detected
	}

	// Load workspace
	workspace, err := workspaceService.LoadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace '%s'", workspaceName)
	}

	deps.Logger.Debug("Loaded workspace info",
		ux.Field("name", workspace.Name),
		ux.Field("path", workspace.Path))

	// Handle field-specific output
	if outputField != "" {
		return printFieldV2(workspace, outputField)
	}

	// Handle JSON output
	if outputFormat == "json" {
		return printWorkspaceJSONV2(workspace)
	}

	// Default table output
	return printInfoTableV2(workspace)
}

func printFieldV2(workspace *domain.Workspace, field string) error {
	switch strings.ToLower(field) {
	case "path":
		fmt.Println(workspace.Path)
	case "name":
		fmt.Println(workspace.Name)
	case "branch":
		fmt.Println(workspace.Branch)
	case "repositories":
		fmt.Println(len(workspace.Repositories))
	case "created":
		fmt.Println(workspace.Created.Format("2006-01-02 15:04:05"))
	case "date":
		fmt.Println(workspace.Created.Format("2006-01-02"))
	case "time":
		fmt.Println(workspace.Created.Format("15:04:05"))
	default:
		return errors.Errorf("unknown field: %s. Available fields: path, name, branch, repositories, created, date, time", field)
	}
	return nil
}

func printWorkspaceJSONV2(workspace *domain.Workspace) error {
	data, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal workspace to JSON")
	}
	fmt.Println(string(data))
	return nil
}

func printInfoTableV2(workspace *domain.Workspace) error {
	fmt.Printf("Workspace Information\n")
	fmt.Printf("====================\n\n")
	fmt.Printf("  Name:         %s\n", workspace.Name)
	fmt.Printf("  Path:         %s\n", workspace.Path)
	fmt.Printf("  Branch:       %s\n", workspace.Branch)
	fmt.Printf("  Repositories: %d\n", len(workspace.Repositories))
	fmt.Printf("  Created:      %s\n", workspace.Created.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Go Workspace: %t\n", workspace.GoWorkspace)

	if len(workspace.Repositories) > 0 {
		fmt.Printf("\nRepositories\n")
		fmt.Printf("============\n")
		for _, repo := range workspace.Repositories {
			fmt.Printf("  - %s (%s)\n", repo.Name, repo.RemoteURL)
		}
	}

	return nil
}
