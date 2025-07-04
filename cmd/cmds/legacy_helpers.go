package cmds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/output"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// detectWorkspace detects the current workspace from the working directory
func detectWorkspace(cwd string) (string, error) {
	log.Debug().Str("cwd", cwd).Msg("Starting workspace detection")

	// First, try to find a workspace that contains this directory
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)
	workspaces, err := workspaceService.ListWorkspaces()
	if err != nil {
		log.Debug().Err(err).Msg("Failed to load workspaces")
		return "", errors.Wrap(err, "failed to load workspaces")
	}

	log.Debug().Int("workspaceCount", len(workspaces)).Msg("Loaded workspaces")

	// Check if current directory is within any workspace path
	for _, workspace := range workspaces {
		log.Debug().
			Str("workspaceName", workspace.Name).
			Str("workspacePath", workspace.Path).
			Msg("Checking workspace")

		// Check if current directory is within or matches workspace path
		if strings.HasPrefix(cwd, workspace.Path) {
			output.LogInfo(
				fmt.Sprintf("Detected workspace: %s", workspace.Name),
				"Found workspace containing current directory",
				"workspaceName", workspace.Name,
				"workspacePath", workspace.Path,
				"cwd", cwd,
			)
			return workspace.Name, nil
		}

		// Also check if any repository in the workspace matches current directory
		for _, repo := range workspace.Repositories {
			repoWorktreePath := filepath.Join(workspace.Path, repo.Name)
			log.Debug().
				Str("repo", repo.Name).
				Str("repoWorktreePath", repoWorktreePath).
				Msg("Checking repository worktree path")

			if strings.HasPrefix(cwd, repoWorktreePath) {
				output.LogInfo(
					fmt.Sprintf("Detected workspace: %s (via repo %s)", workspace.Name, repo.Name),
					"Found workspace via repository worktree path",
					"workspaceName", workspace.Name,
					"repo", repo.Name,
					"repoWorktreePath", repoWorktreePath,
					"cwd", cwd,
				)
				return workspace.Name, nil
			}
		}
	}

	log.Debug().Msg("No workspace found containing current directory, trying heuristic detection")

	// Fallback: Look for workspace configuration file in current directory or parents
	dir := cwd

	for {
		log.Debug().Str("dir", dir).Msg("Checking directory for workspace structure")

		// Check if this directory contains repository worktrees
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Debug().Err(err).Str("dir", dir).Msg("Failed to read directory")
			return "", err
		}

		// Look for .git files (worktree indicators) and workspace structure
		gitDirs := 0
		var gitRepos []string
		for _, entry := range entries {
			if entry.IsDir() {
				gitFile := filepath.Join(dir, entry.Name(), ".git")
				if stat, err := os.Stat(gitFile); err == nil && stat.Mode().IsRegular() {
					gitDirs++
					gitRepos = append(gitRepos, entry.Name())
				}
			}
		}

		log.Debug().
			Str("dir", dir).
			Int("gitDirs", gitDirs).
			Strs("gitRepos", gitRepos).
			Msg("Found git repositories in directory")

		// If we found multiple git worktrees, this might be a workspace
		if gitDirs >= 2 {
			// Try to find a workspace that matches this path
			dirName := filepath.Base(dir)
			log.Debug().Str("dirName", dirName).Msg("Checking if directory name matches any workspace")

			for _, workspace := range workspaces {
				if workspace.Name == dirName || strings.Contains(workspace.Path, dirName) {
					output.LogInfo(
						fmt.Sprintf("Detected workspace: %s", workspace.Name),
						"Found workspace by directory name match",
						"workspaceName", workspace.Name,
						"dirName", dirName,
					)
					return workspace.Name, nil
				}
			}

			// If no exact match, return the directory name as best guess
			log.Debug().Str("dirName", dirName).Msg("Using directory name as workspace name")
			return dirName, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			log.Debug().Msg("Reached filesystem root")
			break // Reached root
		}
		dir = parent
	}

	log.Debug().Msg("No workspace detected")
	return "", errors.New("not in a workspace directory")
}

// loadWorkspace loads a workspace by name
func loadWorkspace(name string) (*domain.Workspace, error) {
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)
	workspaces, err := workspaceService.ListWorkspaces()
	if err != nil {
		return nil, err
	}

	for _, workspace := range workspaces {
		if workspace.Name == name {
			return &workspace, nil
		}
	}

	return nil, errors.Errorf("workspace not found: %s", name)
}
