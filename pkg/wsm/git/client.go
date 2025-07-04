package git

import (
	"context"
)

// Client abstracts git operations for testability and clean separation
type Client interface {
	// Worktree operations
	WorktreeAdd(ctx context.Context, repoPath, branch, targetPath string, opts WorktreeAddOpts) error
	WorktreeRemove(ctx context.Context, repoPath, targetPath string, force bool) error
	WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error)

	// Branch operations
	BranchExists(ctx context.Context, repoPath, branch string) (bool, error)
	RemoteBranchExists(ctx context.Context, repoPath, branch string) (bool, error)
	CurrentBranch(ctx context.Context, repoPath string) (string, error)
	CreateBranch(ctx context.Context, repoPath, branchName string, track bool) error
	SwitchBranch(ctx context.Context, repoPath, branchName string) error

	// Status and changes
	Status(ctx context.Context, repoPath string) (*StatusInfo, error)
	AheadBehind(ctx context.Context, repoPath string) (ahead, behind int, err error)
	HasChanges(ctx context.Context, repoPath string) (bool, error)
	UntrackedFiles(ctx context.Context, repoPath string) ([]string, error)

	// Operations
	Add(ctx context.Context, repoPath, filePath string) error
	Commit(ctx context.Context, repoPath, message string) error
	Push(ctx context.Context, repoPath, remote, branch string) error
	Pull(ctx context.Context, repoPath, remote, branch string) error
	Fetch(ctx context.Context, repoPath, remote string) error
	Checkout(ctx context.Context, repoPath, branch string) error
	Merge(ctx context.Context, repoPath, branch string) error
	ResetHard(ctx context.Context, repoPath, ref string) error

	// Rebase operations
	Rebase(ctx context.Context, repoPath, targetBranch string, interactive bool) error
	GetCommitsAhead(ctx context.Context, repoPath, targetBranch string) (int, error)
	HasRebaseConflicts(ctx context.Context, repoPath string) (bool, error)
	FetchBranch(ctx context.Context, repoPath, branch string) error

	// Repository info
	RemoteURL(ctx context.Context, repoPath string) (string, error)
	Branches(ctx context.Context, repoPath string) ([]string, error)
	Tags(ctx context.Context, repoPath string) ([]string, error)
	LastCommit(ctx context.Context, repoPath string) (string, error)
	IsRepository(ctx context.Context, path string) (bool, error)
}

// WorktreeAddOpts contains options for adding worktrees
type WorktreeAddOpts struct {
	Force     bool
	Track     string
	NewBranch bool
}

// WorktreeInfo contains information about a worktree
type WorktreeInfo struct {
	Path   string
	Branch string
	Commit string
}

// StatusInfo contains git status information
type StatusInfo struct {
	StagedFiles    []string
	ModifiedFiles  []string
	UntrackedFiles []string
	HasConflicts   bool
	Clean          bool
}
