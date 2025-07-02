package status

import (
	"context"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/fs"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/git"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
)

// Service handles workspace and repository status operations
type Service struct {
	fs     fs.FileSystem
	git    git.Client
	logger ux.Logger
}

// New creates a new status service
func New(fileSystem fs.FileSystem, gitClient git.Client, logger ux.Logger) *Service {
	return &Service{
		fs:     fileSystem,
		git:    gitClient,
		logger: logger,
	}
}

// GetWorkspaceStatus gets the comprehensive status of a workspace
func (s *Service) GetWorkspaceStatus(ctx context.Context, workspace domain.Workspace) (*domain.WorkspaceStatus, error) {
	s.logger.Info("Getting workspace status", ux.Field("workspace", workspace.Name))

	var repoStatuses []domain.RepositoryStatus

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		status, err := s.GetRepositoryStatus(ctx, repo, repoPath)
		if err != nil {
			s.logger.Error("Failed to get repository status", 
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
			return nil, errors.Wrapf(err, "failed to get status for repository %s", repo.Name)
		}
		repoStatuses = append(repoStatuses, *status)
	}

	overall := s.calculateOverallStatus(repoStatuses)

	return &domain.WorkspaceStatus{
		Workspace:    workspace,
		Repositories: repoStatuses,
		Overall:      overall,
	}, nil
}

// GetRepositoryStatus gets the detailed git status of a single repository
func (s *Service) GetRepositoryStatus(ctx context.Context, repo domain.Repository, repoPath string) (*domain.RepositoryStatus, error) {
	s.logger.Debug("Getting repository status", 
		ux.Field("repo", repo.Name),
		ux.Field("path", repoPath))

	status := &domain.RepositoryStatus{
		Repository: repo,
	}

	// Get current branch
	if branch, err := s.git.CurrentBranch(ctx, repoPath); err == nil {
		status.CurrentBranch = branch
	} else {
		s.logger.Debug("Failed to get current branch", 
			ux.Field("repo", repo.Name),
			ux.Field("error", err))
	}

	// Get git status information
	gitStatus, err := s.git.Status(ctx, repoPath)
	if err != nil {
		s.logger.Debug("Failed to get git status", 
			ux.Field("repo", repo.Name),
			ux.Field("error", err))
	} else {
		status.StagedFiles = gitStatus.StagedFiles
		status.ModifiedFiles = gitStatus.ModifiedFiles
		status.UntrackedFiles = gitStatus.UntrackedFiles
		status.HasConflicts = gitStatus.HasConflicts
		status.HasChanges = !gitStatus.Clean
	}

	// Get ahead/behind status
	if ahead, behind, err := s.git.AheadBehind(ctx, repoPath); err == nil {
		status.Ahead = ahead
		status.Behind = behind
	} else {
		s.logger.Debug("Failed to get ahead/behind status", 
			ux.Field("repo", repo.Name),
			ux.Field("error", err))
	}

	// Check if branch is merged and needs rebase (these would need additional git operations)
	status.IsMerged = s.checkBranchMerged(ctx, repoPath, status.CurrentBranch)
	status.NeedsRebase = s.checkBranchNeedsRebase(ctx, repoPath, status.CurrentBranch)

	return status, nil
}

// CheckRepositoryExists verifies if a repository worktree exists
func (s *Service) CheckRepositoryExists(workspace domain.Workspace, repoName string) bool {
	repoPath := workspace.RepositoryWorktreePath(repoName)
	return s.fs.Exists(repoPath)
}

// GetCleanRepositories returns repositories with no uncommitted changes
func (s *Service) GetCleanRepositories(ctx context.Context, workspace domain.Workspace) ([]domain.Repository, error) {
	var cleanRepos []domain.Repository

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		hasChanges, err := s.git.HasChanges(ctx, repoPath)
		if err != nil {
			s.logger.Debug("Failed to check for changes", 
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
			continue
		}

		if !hasChanges {
			cleanRepos = append(cleanRepos, repo)
		}
	}

	return cleanRepos, nil
}

// GetDirtyRepositories returns repositories with uncommitted changes
func (s *Service) GetDirtyRepositories(ctx context.Context, workspace domain.Workspace) ([]domain.Repository, error) {
	var dirtyRepos []domain.Repository

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		hasChanges, err := s.git.HasChanges(ctx, repoPath)
		if err != nil {
			s.logger.Debug("Failed to check for changes", 
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
			continue
		}

		if hasChanges {
			dirtyRepos = append(dirtyRepos, repo)
		}
	}

	return dirtyRepos, nil
}

// calculateOverallStatus determines the overall workspace status
func (s *Service) calculateOverallStatus(repoStatuses []domain.RepositoryStatus) string {
	if len(repoStatuses) == 0 {
		return "empty"
	}

	hasChanges := false
	hasConflicts := false
	hasStagedFiles := false
	isAhead := false
	isBehind := false

	for _, status := range repoStatuses {
		if status.HasChanges {
			hasChanges = true
		}
		if status.HasConflicts {
			hasConflicts = true
		}
		if len(status.StagedFiles) > 0 {
			hasStagedFiles = true
		}
		if status.Ahead > 0 {
			isAhead = true
		}
		if status.Behind > 0 {
			isBehind = true
		}
	}

	if hasConflicts {
		return "conflicts"
	}
	if hasStagedFiles {
		return "staged"
	}
	if hasChanges {
		return "dirty"
	}
	if isAhead && isBehind {
		return "diverged"
	}
	if isAhead {
		return "ahead"
	}
	if isBehind {
		return "behind"
	}

	return "clean"
}

// checkBranchMerged checks if the current branch is merged to origin/main
func (s *Service) checkBranchMerged(ctx context.Context, repoPath, branch string) bool {
	// This is a simplified implementation
	// In practice, you'd run: git merge-base --is-ancestor <branch> origin/main
	// For now, return false as a safe default
	return false
}

// checkBranchNeedsRebase checks if the current branch needs to be rebased on origin/main
func (s *Service) checkBranchNeedsRebase(ctx context.Context, repoPath, branch string) bool {
	// This is a simplified implementation
	// In practice, you'd check if origin/main has commits not in the current branch
	// For now, return false as a safe default
	return false
}
