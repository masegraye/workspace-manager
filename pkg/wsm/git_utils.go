package wsm

import (
	"context"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// getGitCurrentBranch returns the current branch name
func getGitCurrentBranch(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// GetGitDefaultBranch returns the default branch name from the remote
func GetGitDefaultBranch(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		// If symbolic-ref fails, try to get it from remote show
		cmd = exec.CommandContext(ctx, "git", "remote", "show", "origin")
		cmd.Dir = path
		output, err = cmd.Output()
		if err != nil {
			// Fallback to main if we can't determine the default branch
			return "main", nil
		}
		
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "HEAD branch:") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					return parts[2], nil
				}
			}
		}
		return "main", nil
	}
	
	// Parse the symbolic-ref output (e.g., "refs/remotes/origin/main")
	ref := strings.TrimSpace(string(output))
	parts := strings.Split(ref, "/")
	if len(parts) >= 3 {
		return parts[len(parts)-1], nil
	}
	
	return "main", nil
}

// CheckBranchMerged checks if the current branch has been merged to the default branch
func CheckBranchMerged(ctx context.Context, path string) (bool, error) {
	// Get current branch for logging
	currentBranch, branchErr := getGitCurrentBranch(ctx, path)
	if branchErr != nil {
		log.Debug().Err(branchErr).Str("path", path).Msg("Failed to get current branch for merge check")
		currentBranch = "unknown"
	}

	// Get the default branch
	defaultBranch, defaultErr := GetGitDefaultBranch(ctx, path)
	if defaultErr != nil {
		log.Debug().Err(defaultErr).Str("path", path).Msg("Failed to get default branch, falling back to main")
		defaultBranch = "main"
	}

	log.Debug().Str("path", path).Str("branch", currentBranch).Str("default_branch", defaultBranch).Msg("Checking if branch is merged to default branch")

	// First, fetch to ensure we have latest remote refs
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", defaultBranch)
	fetchCmd.Dir = path
	fetchErr := fetchCmd.Run()
	if fetchErr != nil {
		log.Debug().Err(fetchErr).Str("path", path).Str("default_branch", defaultBranch).Msg("Failed to fetch origin default branch - might be offline")
	} else {
		log.Debug().Str("path", path).Str("default_branch", defaultBranch).Msg("Successfully fetched origin default branch")
	}

	// Check if HEAD has been merged into origin/defaultBranch
	// This command returns 0 if the current HEAD is merged, non-zero otherwise
	originRef := "origin/" + defaultBranch
	cmd := exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", "HEAD", originRef)
	cmd.Dir = path
	err := cmd.Run()

	merged := err == nil
	log.Debug().Str("path", path).Str("branch", currentBranch).Str("default_branch", defaultBranch).Bool("merged", merged).Msg("Branch merge check result")

	return merged, nil
}

// CheckBranchNeedsRebase checks if the current branch needs to be rebased on the default branch
func CheckBranchNeedsRebase(ctx context.Context, path string) (bool, error) {
	// Get current branch for logging
	currentBranch, branchErr := getGitCurrentBranch(ctx, path)
	if branchErr != nil {
		log.Debug().Err(branchErr).Str("path", path).Msg("Failed to get current branch for rebase check")
		currentBranch = "unknown"
	}

	// Get the default branch
	defaultBranch, defaultErr := GetGitDefaultBranch(ctx, path)
	if defaultErr != nil {
		log.Debug().Err(defaultErr).Str("path", path).Msg("Failed to get default branch, falling back to main")
		defaultBranch = "main"
	}

	// Skip rebase check if we're on the default branch
	if currentBranch == defaultBranch {
		log.Debug().Str("path", path).Str("branch", currentBranch).Str("default_branch", defaultBranch).Msg("Skipping rebase check - already on default branch")
		return false, nil
	}

	log.Debug().Str("path", path).Str("branch", currentBranch).Str("default_branch", defaultBranch).Msg("Checking if branch needs rebase on default branch")

	// First, fetch to ensure we have latest remote refs
	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", defaultBranch)
	fetchCmd.Dir = path
	fetchErr := fetchCmd.Run()
	if fetchErr != nil {
		log.Debug().Err(fetchErr).Str("path", path).Str("default_branch", defaultBranch).Msg("Failed to fetch origin default branch - might be offline")
	} else {
		log.Debug().Str("path", path).Str("default_branch", defaultBranch).Msg("Successfully fetched origin default branch")
	}

	// Check if origin/defaultBranch has new commits compared to the merge-base
	// This tells us if origin/defaultBranch has moved forward since we branched
	originRef := "origin/" + defaultBranch
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", "HEAD.."+originRef)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Str("path", path).Str("default_branch", defaultBranch).Msg("Failed to check for commits ahead on origin default branch")
		return false, err
	}

	commitCount := strings.TrimSpace(string(output))
	needsRebase := commitCount != "0"
	log.Debug().Str("path", path).Str("branch", currentBranch).Str("default_branch", defaultBranch).Str("commits_behind", commitCount).Bool("needs_rebase", needsRebase).Msg("Branch rebase check result")

	return needsRebase, nil
}
