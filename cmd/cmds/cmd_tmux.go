package cmds

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewTmuxCommand() *cobra.Command {
	var workspace string
	var profile string

	cmd := &cobra.Command{
		Use:   "tmux [workspace-name]",
		Short: "Create or attach to a tmux session for the workspace",
		Long: `Create or attach to a tmux session named after the workspace.
If no workspace name is provided, attempts to detect the current workspace.

The command will:
1. Create a new tmux session or attach to existing one with the workspace name
2. Execute commands from tmux.conf files based on profile selection:
   - If --profile is specified: .wsm/profiles/PROFILE/tmux.conf
   - Otherwise: .wsm/tmux.conf (fallback to default behavior)
3. Search both workspace root and all top-level directories`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaceName := workspace
			if len(args) > 0 {
				workspaceName = args[0]
			}
			return runTmux(cmd.Context(), workspaceName, profile)
		},
	}

	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace name")
	cmd.Flags().StringVar(&profile, "profile", "", "Tmux profile to use (looks for .wsm/profiles/PROFILE/tmux.conf)")

	carapace.Gen(cmd).PositionalCompletion(WorkspaceNameCompletion())

	return cmd
}

func runTmux(ctx context.Context, workspaceName, profile string) error {
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
		// Replace current process with tmux attach
		return execTmux("attach-session", "-t", sessionName)
	}

	output.PrintInfo("Creating new tmux session: %s", sessionName)

	// Create new session in detached mode
	createCmd := exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", sessionName, "-c", workspace.Path)
	if err := createCmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to create tmux session '%s'", sessionName)
	}

	// Execute tmux.conf files
	if err := executeTmuxConfFiles(ctx, workspace, sessionName, profile); err != nil {
		log.Warn().Err(err).Msg("Failed to execute tmux.conf files")
	}

	// Replace current process with tmux attach
	return execTmux("attach-session", "-t", sessionName)
}

func executeTmuxConfFiles(ctx context.Context, workspace *wsm.Workspace, sessionName, profile string) error {
	// Determine tmux.conf file paths based on profile
	var tmuxConfPaths []TmuxConfPath

	if profile != "" {
		// Use profile-specific tmux.conf files
		tmuxConfPaths = getTmuxConfPathsForProfile(workspace, profile)
		output.PrintInfo("Using tmux profile: %s", profile)
	} else {
		// Use default tmux.conf files
		tmuxConfPaths = getDefaultTmuxConfPaths(workspace)
	}

	// Execute all found tmux.conf files
	for _, confPath := range tmuxConfPaths {
		if err := executeTmuxConfFile(ctx, confPath.FilePath, sessionName, confPath.WorkingDir); err != nil {
			log.Debug().Err(err).Str("file", confPath.FilePath).Msg("Failed to execute tmux.conf")
		}
	}

	return nil
}

// TmuxConfPath represents a tmux.conf file with its working directory
type TmuxConfPath struct {
	FilePath   string
	WorkingDir string
}

// getTmuxConfPathsForProfile returns profile-specific tmux.conf file paths
func getTmuxConfPathsForProfile(workspace *wsm.Workspace, profile string) []TmuxConfPath {
	var paths []TmuxConfPath

	// Workspace root profile tmux.conf
	rootProfileConf := filepath.Join(workspace.Path, ".wsm", "profiles", profile, "tmux.conf")
	paths = append(paths, TmuxConfPath{
		FilePath:   rootProfileConf,
		WorkingDir: workspace.Path,
	})

	// Repository profile tmux.conf files
	entries, err := os.ReadDir(workspace.Path)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to read workspace directory for profile tmux.conf")
		return paths
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirPath := filepath.Join(workspace.Path, entry.Name())
			profileConfPath := filepath.Join(dirPath, ".wsm", "profiles", profile, "tmux.conf")

			paths = append(paths, TmuxConfPath{
				FilePath:   profileConfPath,
				WorkingDir: dirPath,
			})
		}
	}

	return paths
}

// getDefaultTmuxConfPaths returns default tmux.conf file paths
func getDefaultTmuxConfPaths(workspace *wsm.Workspace) []TmuxConfPath {
	var paths []TmuxConfPath

	// Workspace root tmux.conf
	rootTmuxConf := filepath.Join(workspace.Path, ".wsm", "tmux.conf")
	paths = append(paths, TmuxConfPath{
		FilePath:   rootTmuxConf,
		WorkingDir: workspace.Path,
	})

	// Repository tmux.conf files
	entries, err := os.ReadDir(workspace.Path)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to read workspace directory for default tmux.conf")
		return paths
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirPath := filepath.Join(workspace.Path, entry.Name())
			tmuxConfPath := filepath.Join(dirPath, ".wsm", "tmux.conf")

			paths = append(paths, TmuxConfPath{
				FilePath:   tmuxConfPath,
				WorkingDir: dirPath,
			})
		}
	}

	return paths
}

func executeTmuxConfFile(ctx context.Context, tmuxConfPath, sessionName, workingDir string) error {
	// Check if file exists
	if _, err := os.Stat(tmuxConfPath); os.IsNotExist(err) {
		return nil // File doesn't exist, not an error
	}

	log.Debug().Str("file", tmuxConfPath).Str("session", sessionName).Msg("Executing tmux.conf file")

	file, err := os.Open(tmuxConfPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open tmux.conf file: %s", tmuxConfPath)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Str("file", tmuxConfPath).Msg("Failed to close tmux.conf file")
		}
	}()

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
		return errors.Wrapf(err, "failed to read tmux.conf file: %s", tmuxConfPath)
	}

	return nil
}

// execTmux replaces the current process with tmux, allowing the wsm binary to be overwritten
func execTmux(args ...string) error {
	// Find tmux binary
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return errors.Wrap(err, "tmux not found in PATH")
	}

	// Prepare arguments (tmux + provided args)
	execArgs := append([]string{"tmux"}, args...)

	// Replace current process with tmux
	err = syscall.Exec(tmuxPath, execArgs, os.Environ())
	if err != nil {
		return errors.Wrap(err, "failed to exec tmux")
	}

	// This line should never be reached
	return nil
}
