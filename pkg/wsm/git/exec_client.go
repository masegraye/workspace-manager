package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ExecClient implements the Git Client interface using exec.Command
type ExecClient struct{}

func NewExecClient() Client {
	return &ExecClient{}
}

func (c *ExecClient) WorktreeAdd(ctx context.Context, repoPath, branch, targetPath string, opts WorktreeAddOpts) error {
	args := []string{"worktree", "add"}

	if opts.Force {
		args = append(args, "-f")
	}

	if opts.NewBranch && branch != "" {
		args = append(args, "-b", branch)
	}

	args = append(args, targetPath)

	if opts.Track != "" {
		args = append(args, opts.Track)
	} else if branch != "" && !opts.NewBranch {
		args = append(args, branch)
	}

	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) WorktreeRemove(ctx context.Context, repoPath, targetPath string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, targetPath)

	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	return c.parseWorktreeList(output), nil
}

func (c *ExecClient) BranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	err := c.runInRepo(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		if isExitError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *ExecClient) RemoteBranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	err := c.runInRepo(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	if err != nil {
		if isExitError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *ExecClient) CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *ExecClient) CreateBranch(ctx context.Context, repoPath, branchName string, track bool) error {
	args := []string{"checkout", "-b", branchName}
	if track {
		args = append(args, "--track")
	}
	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) SwitchBranch(ctx context.Context, repoPath, branchName string) error {
	return c.runInRepo(ctx, repoPath, "git", "checkout", branchName)
}

func (c *ExecClient) Status(ctx context.Context, repoPath string) (*StatusInfo, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	return c.parseStatus(output), nil
}

func (c *ExecClient) AheadBehind(ctx context.Context, repoPath string) (int, int, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected output format: %s", output)
	}

	ahead, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}

	behind, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}

	return ahead, behind, nil
}

func (c *ExecClient) HasChanges(ctx context.Context, repoPath string) (bool, error) {
	status, err := c.Status(ctx, repoPath)
	if err != nil {
		return false, err
	}
	return !status.Clean, nil
}

func (c *ExecClient) UntrackedFiles(ctx context.Context, repoPath string) ([]string, error) {
	status, err := c.Status(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	return status.UntrackedFiles, nil
}

func (c *ExecClient) Add(ctx context.Context, repoPath, filePath string) error {
	return c.runInRepo(ctx, repoPath, "git", "add", filePath)
}

func (c *ExecClient) Commit(ctx context.Context, repoPath, message string) error {
	return c.runInRepo(ctx, repoPath, "git", "commit", "-m", message)
}

func (c *ExecClient) Push(ctx context.Context, repoPath, remote, branch string) error {
	args := []string{"push"}
	if remote != "" {
		args = append(args, remote)
	}
	if branch != "" {
		args = append(args, branch)
	}
	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) Pull(ctx context.Context, repoPath, remote, branch string) error {
	args := []string{"pull"}
	if remote != "" {
		args = append(args, remote)
	}
	if branch != "" {
		args = append(args, branch)
	}
	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) Fetch(ctx context.Context, repoPath, remote string) error {
	args := []string{"fetch"}
	if remote != "" {
		args = append(args, remote)
	}
	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) Checkout(ctx context.Context, repoPath, branch string) error {
	return c.runInRepo(ctx, repoPath, "git", "checkout", branch)
}

func (c *ExecClient) Merge(ctx context.Context, repoPath, branch string) error {
	return c.runInRepo(ctx, repoPath, "git", "merge", branch)
}

func (c *ExecClient) ResetHard(ctx context.Context, repoPath, ref string) error {
	return c.runInRepo(ctx, repoPath, "git", "reset", "--hard", ref)
}

func (c *ExecClient) RemoteURL(ctx context.Context, repoPath string) (string, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *ExecClient) Branches(ctx context.Context, repoPath string) ([]string, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var branches []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func (c *ExecClient) Tags(ctx context.Context, repoPath string) ([]string, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "tag", "-l")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var tags []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			tags = append(tags, line)
		}
	}
	return tags, nil
}

func (c *ExecClient) LastCommit(ctx context.Context, repoPath string) (string, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *ExecClient) IsRepository(ctx context.Context, path string) (bool, error) {
	err := c.runInRepo(ctx, path, "git", "rev-parse", "--git-dir")
	if err != nil {
		if isExitError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Helper methods

func (c *ExecClient) runInRepo(ctx context.Context, repoPath string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = repoPath
	return cmd.Run()
}

func (c *ExecClient) runInRepoWithOutput(ctx context.Context, repoPath string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	return string(output), err
}

func (c *ExecClient) parseWorktreeList(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	var current WorktreeInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}

	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

func (c *ExecClient) parseStatus(output string) *StatusInfo {
	status := &StatusInfo{
		Clean: true,
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}

		status.Clean = false

		if len(line) < 3 {
			continue
		}

		indexStatus := line[0]
		workingStatus := line[1]
		fileName := line[3:]

		switch indexStatus {
		case 'A', 'M', 'D', 'R', 'C':
			status.StagedFiles = append(status.StagedFiles, fileName)
		}

		switch workingStatus {
		case 'M', 'D':
			status.ModifiedFiles = append(status.ModifiedFiles, fileName)
		case '?':
			status.UntrackedFiles = append(status.UntrackedFiles, fileName)
		}

		if indexStatus == 'U' || workingStatus == 'U' {
			status.HasConflicts = true
		}
	}

	return status
}

// Rebase operations
func (c *ExecClient) Rebase(ctx context.Context, repoPath, targetBranch string, interactive bool) error {
	args := []string{"rebase"}
	if interactive {
		args = append(args, "-i")
	}
	args = append(args, targetBranch)

	return c.runInRepo(ctx, repoPath, "git", args...)
}

func (c *ExecClient) GetCommitsAhead(ctx context.Context, repoPath, targetBranch string) (int, error) {
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "rev-list", "--count", fmt.Sprintf("HEAD..%s", targetBranch))
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, fmt.Errorf("failed to parse commit count: %w", err)
	}

	return count, nil
}

func (c *ExecClient) HasRebaseConflicts(ctx context.Context, repoPath string) (bool, error) {
	// Check git status for conflict markers
	output, err := c.runInRepoWithOutput(ctx, repoPath, "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if len(line) >= 2 && (line[0] == 'U' || line[1] == 'U' ||
			(line[0] == 'A' && line[1] == 'A') ||
			(line[0] == 'D' && line[1] == 'D')) {
			return true, nil
		}
	}

	// Check for rebase directories
	rebaseMergePath := filepath.Join(repoPath, ".git", "rebase-merge")
	rebaseApplyPath := filepath.Join(repoPath, ".git", "rebase-apply")

	if _, err := os.Stat(rebaseMergePath); err == nil {
		return true, nil
	}
	if _, err := os.Stat(rebaseApplyPath); err == nil {
		return true, nil
	}

	return false, nil
}

func (c *ExecClient) FetchBranch(ctx context.Context, repoPath, branch string) error {
	return c.runInRepo(ctx, repoPath, "git", "fetch", "origin", fmt.Sprintf("%s:%s", branch, branch))
}

func isExitError(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}
