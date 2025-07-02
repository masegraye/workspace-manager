package worktree

import (
	"context"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/git"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
)

// Service handles worktree operations
type Service struct {
	git git.Client
	log ux.Logger
}

// New creates a new worktree service
func New(gitClient git.Client, logger ux.Logger) *Service {
	return &Service{
		git: gitClient,
		log: logger,
	}
}

// CreateOpts contains options for creating worktrees
type CreateOpts struct {
	Force        bool
	TrackRemote  bool
	RemoteExists bool // hint to avoid extra git calls
}

// Create creates a new worktree for the repository at the target path
func (s *Service) Create(ctx context.Context, repo domain.Repository, targetPath, branch string, opts CreateOpts) error {
	s.log.Info("Creating worktree",
		ux.Field("repo", repo.Name),
		ux.Field("branch", branch),
		ux.Field("target", targetPath))

	// If force is enabled, create branch and worktree forcefully
	if opts.Force {
		return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{
			Force:     true,
			NewBranch: true,
		})
	}

	// Check if branch exists locally
	localExists, err := s.git.BranchExists(ctx, repo.Path, branch)
	if err != nil {
		s.log.Error("Failed to check if local branch exists",
			ux.Field("repo", repo.Name),
			ux.Field("branch", branch),
			ux.Field("error", err))
		return err
	}

	// Check if branch exists on remote (if not provided as hint)
	var remoteExists bool
	if opts.RemoteExists {
		remoteExists = true
	} else {
		remoteExists, err = s.git.RemoteBranchExists(ctx, repo.Path, branch)
		if err != nil {
			s.log.Error("Failed to check if remote branch exists",
				ux.Field("repo", repo.Name),
				ux.Field("branch", branch),
				ux.Field("error", err))
			return err
		}
	}

	// Determine strategy based on branch existence
	if localExists {
		s.log.Debug("Using existing local branch",
			ux.Field("repo", repo.Name),
			ux.Field("branch", branch))
		return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{})
	} else if remoteExists {
		s.log.Debug("Tracking remote branch",
			ux.Field("repo", repo.Name),
			ux.Field("branch", branch))
		return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{
			Track: "origin/" + branch,
		})
	} else {
		s.log.Debug("Creating new branch",
			ux.Field("repo", repo.Name),
			ux.Field("branch", branch))
		return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{
			NewBranch: true,
		})
	}
}

// Remove removes a worktree at the target path
func (s *Service) Remove(ctx context.Context, repo domain.Repository, targetPath string, force bool) error {
	s.log.Info("Removing worktree",
		ux.Field("repo", repo.Name),
		ux.Field("target", targetPath),
		ux.Field("force", force))

	return s.git.WorktreeRemove(ctx, repo.Path, targetPath, force)
}

// List returns all worktrees for a repository
func (s *Service) List(ctx context.Context, repo domain.Repository) ([]git.WorktreeInfo, error) {
	s.log.Debug("Listing worktrees", ux.Field("repo", repo.Name))
	return s.git.WorktreeList(ctx, repo.Path)
}
