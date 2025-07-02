package cmds

import (
	"context"
	"os"

	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewTmuxCommandV2() *cobra.Command {
	var workspace string
	var profile string

	cmd := &cobra.Command{
		Use:   "tmux-v2 [workspace-name]",
		Short: "Create or attach to a tmux session for the workspace using the new service architecture",
		Long: `Create or attach to a tmux session named after the workspace using the new service architecture.

The new architecture provides:
- Better error handling and logging
- Structured service dependencies
- Clean separation of concerns
- Consistent patterns with other v2 commands

If no workspace name is provided, attempts to detect the current workspace.

The command will:
1. Create a new tmux session or attach to existing one with the workspace name
2. Execute commands from tmux.conf files based on profile selection:
   - If --profile is specified: .wsm/profiles/PROFILE/tmux.conf
   - Otherwise: .wsm/tmux.conf (fallback to default behavior)
3. Search both workspace root and all top-level directories

Examples:
  # Create/attach to tmux session for current workspace
  wsm tmux-v2

  # Create/attach to tmux session for specific workspace  
  wsm tmux-v2 my-workspace

  # Use a specific tmux profile
  wsm tmux-v2 --profile development

  # Specify workspace explicitly
  wsm tmux-v2 --workspace my-workspace`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := workspace
			if len(args) > 0 {
				workspaceName = args[0]
			}
			return runTmuxV2(cmd.Context(), workspaceName, profile)
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name")
	cmd.Flags().StringVar(&profile, "profile", "", "Tmux profile to use (looks for .wsm/profiles/PROFILE/tmux.conf)")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runTmuxV2(ctx context.Context, workspaceName, profile string) error {
	// Initialize the new service architecture
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
			return errors.Wrap(err, "failed to detect workspace. Use 'wsm tmux-v2 <workspace-name>' or specify --workspace flag")
		}
		workspaceName = detected
	}

	deps.Logger.Info("Starting tmux session for workspace",
		ux.Field("workspace", workspaceName),
		ux.Field("profile", profile))

	// Create tmux session using the service
	req := service.TmuxSessionRequest{
		WorkspaceName: workspaceName,
		Profile:       profile,
		SessionName:   workspaceName, // Use workspace name as session name
	}

	return workspaceService.CreateTmuxSession(ctx, req)
}
