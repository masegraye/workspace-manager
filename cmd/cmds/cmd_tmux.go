package cmds

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewTmuxCommand() *cobra.Command {
	var workspace string

	cmd := &cobra.Command{
		Use:   "tmux [workspace-name]",
		Short: "Create or attach to a tmux session for the workspace",
		Long: `Create or attach to a tmux session named after the workspace.
If no workspace name is provided, attempts to detect the current workspace.

The command will:
1. Create a new tmux session or attach to existing one with the workspace name
2. Execute commands from .wsm/tmuxrc in the workspace root (if exists)
3. Execute commands from .wsm/tmuxrc in all top-level directories (if exists)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := workspace
			if len(args) > 0 {
				workspaceName = args[0]
			}
			return runTmux(cmd.Context(), workspaceName)
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runTmux(ctx context.Context, workspaceName string) error {
	// If no workspace specified, try to detect current workspace
	if workspaceName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}

		detected, err := detectWorkspace(cwd)
		if err != nil {
			return errors.Wrap(err, "failed to detect workspace. Use 'wsm tmux <workspace-name>' or specify --workspace flag")
		}
		workspaceName = detected
	}

	// Load workspace
	workspace, err := loadWorkspace(workspaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace '%s'", workspaceName)
	}

	sessionName := workspaceName

	// Check if tmux session already exists
	checkCmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", sessionName)
	sessionExists := checkCmd.Run() == nil

	if sessionExists {
		output.PrintInfo("Attaching to existing tmux session: %s", sessionName)
		// Attach to existing session
		attachCmd := exec.Command("tmux", "attach-session", "-t", sessionName)
		attachCmd.Stdin = os.Stdin
		attachCmd.Stdout = os.Stdout
		attachCmd.Stderr = os.Stderr
		return attachCmd.Run()
	}

	output.PrintInfo("Creating new tmux session: %s", sessionName)

	// Create new session in detached mode
	createCmd := exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", sessionName, "-c", workspace.Path)
	if err := createCmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to create tmux session '%s'", sessionName)
	}

	// Execute tmuxrc files
	if err := executeTmuxrcFiles(ctx, workspace, sessionName); err != nil {
		log.Warn().Err(err).Msg("Failed to execute tmuxrc files")
	}

	// Attach to the session
	attachCmd := exec.Command("tmux", "attach-session", "-t", sessionName)
	attachCmd.Stdin = os.Stdin
	attachCmd.Stdout = os.Stdout
	attachCmd.Stderr = os.Stderr
	return attachCmd.Run()
}

func executeTmuxrcFiles(ctx context.Context, workspace *wsm.Workspace, sessionName string) error {
	// Execute workspace root .wsm/tmuxrc
	rootTmuxrc := filepath.Join(workspace.Path, ".wsm", "tmuxrc")
	if err := executeTmuxrcFile(ctx, rootTmuxrc, sessionName, workspace.Path); err != nil {
		log.Debug().Err(err).Str("file", rootTmuxrc).Msg("Failed to execute root tmuxrc")
	}

	// Execute .wsm/tmuxrc files in all top-level directories
	entries, err := os.ReadDir(workspace.Path)
	if err != nil {
		return errors.Wrap(err, "failed to read workspace directory")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirPath := filepath.Join(workspace.Path, entry.Name())
			tmuxrcPath := filepath.Join(dirPath, ".wsm", "tmuxrc")
			
			if err := executeTmuxrcFile(ctx, tmuxrcPath, sessionName, dirPath); err != nil {
				log.Debug().Err(err).Str("file", tmuxrcPath).Msg("Failed to execute directory tmuxrc")
			}
		}
	}

	return nil
}

func executeTmuxrcFile(ctx context.Context, tmuxrcPath, sessionName, workingDir string) error {
	// Check if file exists
	if _, err := os.Stat(tmuxrcPath); os.IsNotExist(err) {
		return nil // File doesn't exist, not an error
	}

	log.Debug().Str("file", tmuxrcPath).Str("session", sessionName).Msg("Executing tmuxrc file")

	file, err := os.Open(tmuxrcPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open tmuxrc file: %s", tmuxrcPath)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		log.Debug().Str("command", line).Str("session", sessionName).Msg("Executing tmux command")

		// Execute tmux command in the session
		cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, line, "Enter")
		cmd.Dir = workingDir
		
		if err := cmd.Run(); err != nil {
			log.Warn().Err(err).Str("command", line).Msg("Failed to execute tmux command")
			// Continue with other commands even if one fails
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "failed to read tmuxrc file: %s", tmuxrcPath)
	}

	return nil
}
