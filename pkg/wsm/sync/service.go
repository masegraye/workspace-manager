package sync

import (
	"context"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/git"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
)

// Service handles synchronization operations across repositories
type Service struct {
	git    git.Client
	logger ux.Logger
}

// New creates a new sync service
func New(gitClient git.Client, logger ux.Logger) *Service {
	return &Service{
		git:    gitClient,
		logger: logger,
	}
}

// SyncResult represents the result of a sync operation on a repository
type SyncResult struct {
	Repository   string `json:"repository"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
	Pulled       bool   `json:"pulled"`
	Pushed       bool   `json:"pushed"`
	Conflicts    bool   `json:"conflicts"`
	AheadBefore  int    `json:"ahead_before"`
	BehindBefore int    `json:"behind_before"`
	AheadAfter   int    `json:"ahead_after"`
	BehindAfter  int    `json:"behind_after"`
}

// SyncOptions configures sync operations
type SyncOptions struct {
	Pull   bool `json:"pull"`
	Push   bool `json:"push"`
	Rebase bool `json:"rebase"`
	DryRun bool `json:"dry_run"`
}

// SyncWorkspace synchronizes all repositories in the workspace
func (s *Service) SyncWorkspace(ctx context.Context, workspace domain.Workspace, options SyncOptions) ([]SyncResult, error) {
	s.logger.Info("Starting workspace sync",
		ux.Field("workspace", workspace.Name),
		ux.Field("pull", options.Pull),
		ux.Field("push", options.Push),
		ux.Field("rebase", options.Rebase),
		ux.Field("dryRun", options.DryRun))

	var results []SyncResult

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		result := s.syncRepository(ctx, repo.Name, repoPath, options)
		results = append(results, result)
	}

	s.logger.Info("Workspace sync completed",
		ux.Field("workspace", workspace.Name),
		ux.Field("repositories", len(results)))

	return results, nil
}

// SyncRepository synchronizes a single repository
func (s *Service) SyncRepository(ctx context.Context, repoName, repoPath string, options SyncOptions) SyncResult {
	return s.syncRepository(ctx, repoName, repoPath, options)
}

// PullRepository pulls changes from remote
func (s *Service) PullRepository(ctx context.Context, repoPath string, rebase bool) error {
	s.logger.Info("Pulling repository",
		ux.Field("path", repoPath),
		ux.Field("rebase", rebase))

	if err := s.git.Pull(ctx, repoPath, "origin", ""); err != nil {
		s.logger.Error("Failed to pull repository",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		return errors.Wrap(err, "pull failed")
	}

	return nil
}

// PushRepository pushes changes to remote
func (s *Service) PushRepository(ctx context.Context, repoPath string) error {
	s.logger.Info("Pushing repository", ux.Field("path", repoPath))

	if err := s.git.Push(ctx, repoPath, "origin", ""); err != nil {
		s.logger.Error("Failed to push repository",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		return errors.Wrap(err, "push failed")
	}

	return nil
}

// FetchRepository fetches changes from remote without merging
func (s *Service) FetchRepository(ctx context.Context, repoPath string) error {
	s.logger.Info("Fetching repository", ux.Field("path", repoPath))

	if err := s.git.Fetch(ctx, repoPath, "origin"); err != nil {
		s.logger.Error("Failed to fetch repository",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		return errors.Wrap(err, "fetch failed")
	}

	return nil
}

// FetchWorkspace fetches all repositories in the workspace
func (s *Service) FetchWorkspace(ctx context.Context, workspace domain.Workspace) error {
	s.logger.Info("Fetching workspace", ux.Field("workspace", workspace.Name))

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		if err := s.FetchRepository(ctx, repoPath); err != nil {
			s.logger.Error("Failed to fetch repository in workspace",
				ux.Field("workspace", workspace.Name),
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
			return errors.Wrapf(err, "failed to fetch repository %s", repo.Name)
		}
	}

	s.logger.Info("Workspace fetch completed", ux.Field("workspace", workspace.Name))
	return nil
}

// BranchResult represents the result of a branch operation on a repository
type BranchResult struct {
	Repository string `json:"repository"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
	Branch     string `json:"branch"`
}

// CreateBranchWorkspace creates a branch in all repositories in the workspace
func (s *Service) CreateBranchWorkspace(ctx context.Context, workspace domain.Workspace, branchName string, track bool) ([]BranchResult, error) {
	s.logger.Info("Creating branch across workspace",
		ux.Field("workspace", workspace.Name),
		ux.Field("branch", branchName),
		ux.Field("track", track))

	var results []BranchResult

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		result := s.createBranchInRepository(ctx, repo.Name, repoPath, branchName, track)
		results = append(results, result)
	}

	s.logger.Info("Branch creation completed",
		ux.Field("workspace", workspace.Name),
		ux.Field("branch", branchName),
		ux.Field("repositories", len(results)))

	return results, nil
}

// SwitchBranchWorkspace switches to a branch in all repositories in the workspace
func (s *Service) SwitchBranchWorkspace(ctx context.Context, workspace domain.Workspace, branchName string) ([]BranchResult, error) {
	s.logger.Info("Switching branch across workspace",
		ux.Field("workspace", workspace.Name),
		ux.Field("branch", branchName))

	var results []BranchResult

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		result := s.switchBranchInRepository(ctx, repo.Name, repoPath, branchName)
		results = append(results, result)
	}

	s.logger.Info("Branch switch completed",
		ux.Field("workspace", workspace.Name),
		ux.Field("branch", branchName),
		ux.Field("repositories", len(results)))

	return results, nil
}

// ListBranchesWorkspace lists current branches across all repositories in the workspace
func (s *Service) ListBranchesWorkspace(ctx context.Context, workspace domain.Workspace) ([]BranchResult, error) {
	s.logger.Info("Listing branches across workspace", ux.Field("workspace", workspace.Name))

	var results []BranchResult

	for _, repo := range workspace.Repositories {
		repoPath := workspace.RepositoryWorktreePath(repo.Name)
		result := s.getCurrentBranchInRepository(ctx, repo.Name, repoPath)
		results = append(results, result)
	}

	return results, nil
}

// Helper functions

func (s *Service) createBranchInRepository(ctx context.Context, repoName, repoPath, branchName string, track bool) BranchResult {
	result := BranchResult{
		Repository: repoName,
		Branch:     branchName,
		Success:    true,
	}

	if err := s.git.CreateBranch(ctx, repoPath, branchName, track); err != nil {
		s.logger.Error("Failed to create branch",
			ux.Field("repo", repoName),
			ux.Field("branch", branchName),
			ux.Field("error", err))
		result.Success = false
		result.Error = err.Error()
		return result
	}

	s.logger.Debug("Branch created successfully",
		ux.Field("repo", repoName),
		ux.Field("branch", branchName))

	return result
}

func (s *Service) switchBranchInRepository(ctx context.Context, repoName, repoPath, branchName string) BranchResult {
	result := BranchResult{
		Repository: repoName,
		Branch:     branchName,
		Success:    true,
	}

	if err := s.git.SwitchBranch(ctx, repoPath, branchName); err != nil {
		s.logger.Error("Failed to switch branch",
			ux.Field("repo", repoName),
			ux.Field("branch", branchName),
			ux.Field("error", err))
		result.Success = false
		result.Error = err.Error()
		return result
	}

	s.logger.Debug("Branch switched successfully",
		ux.Field("repo", repoName),
		ux.Field("branch", branchName))

	return result
}

func (s *Service) getCurrentBranchInRepository(ctx context.Context, repoName, repoPath string) BranchResult {
	result := BranchResult{
		Repository: repoName,
		Success:    true,
	}

	branch, err := s.git.CurrentBranch(ctx, repoPath)
	if err != nil {
		s.logger.Error("Failed to get current branch",
			ux.Field("repo", repoName),
			ux.Field("error", err))
		result.Success = false
		result.Error = err.Error()
		return result
	}

	result.Branch = branch
	return result
}

// syncRepository synchronizes a single repository (internal implementation)
func (s *Service) syncRepository(ctx context.Context, repoName, repoPath string, options SyncOptions) SyncResult {
	result := SyncResult{
		Repository: repoName,
		Success:    true,
	}

	s.logger.Debug("Syncing repository",
		ux.Field("repo", repoName),
		ux.Field("path", repoPath))

	// Get initial ahead/behind status
	ahead, behind, err := s.git.AheadBehind(ctx, repoPath)
	if err != nil {
		s.logger.Error("Failed to get initial ahead/behind status",
			ux.Field("repo", repoName),
			ux.Field("error", err))
		result.Success = false
		result.Error = "failed to get initial status: " + err.Error()
		return result
	}
	result.AheadBefore = ahead
	result.BehindBefore = behind

	if options.DryRun {
		s.logger.Info("Dry run mode - skipping actual sync", ux.Field("repo", repoName))
		result.Error = "dry-run mode"
		return result
	}

	// Pull changes if requested
	if options.Pull {
		if err := s.git.Pull(ctx, repoPath, "origin", ""); err != nil {
			s.logger.Error("Pull failed",
				ux.Field("repo", repoName),
				ux.Field("error", err))
			result.Success = false
			result.Error = "pull failed: " + err.Error()

			// Check for conflicts
			if status, statusErr := s.git.Status(ctx, repoPath); statusErr == nil {
				result.Conflicts = status.HasConflicts
			}
			return result
		}
		result.Pulled = true
		s.logger.Debug("Pull completed", ux.Field("repo", repoName))
	}

	// Push changes if requested
	if options.Push {
		if err := s.git.Push(ctx, repoPath, "origin", ""); err != nil {
			s.logger.Error("Push failed",
				ux.Field("repo", repoName),
				ux.Field("error", err))
			result.Success = false
			result.Error = "push failed: " + err.Error()
			return result
		}
		result.Pushed = true
		s.logger.Debug("Push completed", ux.Field("repo", repoName))
	}

	// Get final ahead/behind status
	aheadAfter, behindAfter, err := s.git.AheadBehind(ctx, repoPath)
	if err != nil {
		s.logger.Warn("Failed to get final ahead/behind status",
			ux.Field("repo", repoName),
			ux.Field("error", err))
	} else {
		result.AheadAfter = aheadAfter
		result.BehindAfter = behindAfter
	}

	s.logger.Debug("Repository sync completed",
		ux.Field("repo", repoName),
		ux.Field("success", result.Success))

	return result
}
