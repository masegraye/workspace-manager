package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/config"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/discovery"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/gowork"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/metadata"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/status"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/sync"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/worktree"
	"github.com/pkg/errors"
)

// WorkspaceService orchestrates workspace operations
type WorkspaceService struct {
	deps      *Deps
	worktree  *worktree.Service
	metadata  *metadata.Builder
	gowork    *gowork.Generator
	config    *config.Service
	discovery *discovery.Service
	status    *status.Service
	sync      *sync.Service
}

// NewWorkspaceService creates a new workspace service
func NewWorkspaceService(deps *Deps) *WorkspaceService {
	configService := config.New(deps.FS)
	return &WorkspaceService{
		deps:      deps,
		worktree:  worktree.New(deps.Git, deps.Logger),
		metadata:  metadata.New(deps.Clock),
		gowork:    gowork.New(),
		config:    configService,
		discovery: discovery.New(deps.FS, deps.Git, deps.Logger, configService),
		status:    status.New(deps.FS, deps.Git, deps.Logger),
		sync:      sync.New(deps.Git, deps.Logger),
	}
}

// CreateRequest contains parameters for creating a workspace
type CreateRequest struct {
	Name       string
	RepoNames  []string
	Branch     string
	BaseBranch string
	AgentMD    string
	DryRun     bool
}

// CreateOption is a functional option for customizing workspace creation
type CreateOption func(*CreateRequest)

// WithBranch sets the branch for the workspace
func WithBranch(branch string) CreateOption {
	return func(r *CreateRequest) { r.Branch = branch }
}

// WithBaseBranch sets the base branch for the workspace
func WithBaseBranch(baseBranch string) CreateOption {
	return func(r *CreateRequest) { r.BaseBranch = baseBranch }
}

// WithAgentMD sets the AGENT.md content for the workspace
func WithAgentMD(agentMD string) CreateOption {
	return func(r *CreateRequest) { r.AgentMD = agentMD }
}

// DryRun enables dry run mode
func DryRun(enabled bool) CreateOption {
	return func(r *CreateRequest) { r.DryRun = enabled }
}

// Create creates a new workspace
func (s *WorkspaceService) Create(ctx context.Context, req CreateRequest, opts ...CreateOption) (*domain.Workspace, error) {
	// Apply options
	for _, opt := range opts {
		opt(&req)
	}

	s.deps.Logger.Info("Creating workspace", ux.Field("name", req.Name))

	// Validate input
	if req.Name == "" {
		return nil, errors.New("workspace name is required")
	}

	// Load configuration
	cfg, err := s.config.Load()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	workspacePath := s.deps.FS.Join(cfg.WorkspaceDir, req.Name)

	// Find repositories using discovery service
	repos, err := s.discovery.FindRepositories(req.RepoNames)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find repositories")
	}

	// Build workspace object
	workspace := &domain.Workspace{
		Name:         req.Name,
		Path:         workspacePath,
		Repositories: repos,
		Branch:       req.Branch,
		BaseBranch:   req.BaseBranch,
		Created:      s.deps.Clock(),
		GoWorkspace:  s.shouldCreateGoWorkspace(repos),
		AgentMD:      req.AgentMD,
	}

	if req.DryRun {
		s.deps.Logger.Info("Dry run mode - skipping actual creation", ux.Field("workspace", workspace.Name))
		return workspace, nil
	}

	// Create physical structure
	if err := s.createPhysicalStructure(ctx, workspace); err != nil {
		return nil, errors.Wrap(err, "failed to create workspace structure")
	}

	// Save workspace metadata using the config service
	if err := s.config.SaveWorkspace(workspace); err != nil {
		return nil, errors.Wrap(err, "failed to save workspace")
	}

	s.deps.Logger.Info("Workspace created successfully",
		ux.Field("name", workspace.Name),
		ux.Field("path", workspace.Path))

	return workspace, nil
}

// Delete removes a workspace
func (s *WorkspaceService) Delete(ctx context.Context, workspacePath string, force bool) error {
	s.deps.Logger.Info("Deleting workspace",
		ux.Field("path", workspacePath),
		ux.Field("force", force))

	// TODO: Load workspace metadata to get repository info for proper cleanup
	// For now, just remove the directory
	if err := s.deps.FS.RemoveAll(workspacePath); err != nil {
		return errors.Wrap(err, "failed to remove workspace directory")
	}

	s.deps.Logger.Info("Workspace deleted successfully", ux.Field("path", workspacePath))
	return nil
}

// createPhysicalStructure creates the workspace directory structure
func (s *WorkspaceService) createPhysicalStructure(ctx context.Context, ws *domain.Workspace) error {
	// Create workspace directory
	if err := s.deps.FS.MkdirAll(ws.Path, 0755); err != nil {
		return errors.Wrap(err, "failed to create workspace directory")
	}

	// Track created worktrees for rollback
	var created []domain.Repository

	// Create worktrees for each repository
	for _, repo := range ws.Repositories {
		targetPath := ws.RepositoryWorktreePath(repo.Name)

		if err := s.worktree.Create(ctx, repo, targetPath, ws.Branch, worktree.CreateOpts{}); err != nil {
			s.rollback(ctx, ws.Path, created)
			return errors.Wrapf(err, "failed to create worktree for %s", repo.Name)
		}

		created = append(created, repo)
		s.deps.Logger.Debug("Created worktree",
			ux.Field("repo", repo.Name),
			ux.Field("target", targetPath))
	}

	// Create go.work file if needed
	if ws.GoWorkspace {
		content := s.gowork.Generate(*ws)
		if err := s.deps.FS.WriteFile(ws.GoWorkPath(), []byte(content), 0644); err != nil {
			s.rollback(ctx, ws.Path, created)
			return errors.Wrap(err, "failed to create go.work file")
		}
		s.deps.Logger.Debug("Created go.work file", ux.Field("path", ws.GoWorkPath()))
	}

	// Create workspace metadata
	metadataBytes, err := s.metadata.BuildWorkspaceMetadata(*ws)
	if err != nil {
		s.rollback(ctx, ws.Path, created)
		return errors.Wrap(err, "failed to build metadata")
	}

	metadataDir := filepath.Dir(ws.MetadataPath())
	if err := s.deps.FS.MkdirAll(metadataDir, 0755); err != nil {
		s.rollback(ctx, ws.Path, created)
		return errors.Wrap(err, "failed to create metadata directory")
	}

	if err := s.deps.FS.WriteFile(ws.MetadataPath(), metadataBytes, 0644); err != nil {
		s.rollback(ctx, ws.Path, created)
		return errors.Wrap(err, "failed to write metadata")
	}
	s.deps.Logger.Debug("Created workspace metadata", ux.Field("path", ws.MetadataPath()))

	// Create AGENT.md if provided
	if ws.AgentMD != "" {
		if err := s.deps.FS.WriteFile(ws.AgentMDPath(), []byte(ws.AgentMD), 0644); err != nil {
			s.rollback(ctx, ws.Path, created)
			return errors.Wrap(err, "failed to create AGENT.md")
		}
		s.deps.Logger.Debug("Created AGENT.md file", ux.Field("path", ws.AgentMDPath()))
	}

	return nil
}

// rollback removes created worktrees and workspace directory on failure
func (s *WorkspaceService) rollback(ctx context.Context, workspacePath string, created []domain.Repository) {
	s.deps.Logger.Warn("Rolling back workspace creation",
		ux.Field("workspace", workspacePath),
		ux.Field("worktrees", len(created)))

	for _, repo := range created {
		targetPath := s.deps.FS.Join(workspacePath, repo.Name)
		if err := s.worktree.Remove(ctx, repo, targetPath, true); err != nil {
			s.deps.Logger.Error("Failed to rollback worktree",
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
		}
	}

	if err := s.deps.FS.RemoveAll(workspacePath); err != nil {
		s.deps.Logger.Error("Failed to remove workspace directory",
			ux.Field("path", workspacePath),
			ux.Field("error", err))
	}
}

// shouldCreateGoWorkspace determines if a go.work file should be created
func (s *WorkspaceService) shouldCreateGoWorkspace(repos []domain.Repository) bool {
	for _, repo := range repos {
		if repo.IsGoProject() {
			return true
		}
	}
	return false
}

// DiscoverRepositories discovers repositories in the given paths and updates the registry
func (s *WorkspaceService) DiscoverRepositories(ctx context.Context, paths []string, recursive bool, maxDepth int) error {
	repos, err := s.discovery.Discover(ctx, discovery.DiscoverOptions{
		Paths:     paths,
		Recursive: recursive,
		MaxDepth:  maxDepth,
	})
	if err != nil {
		return errors.Wrap(err, "failed to discover repositories")
	}

	return s.discovery.UpdateRegistry(repos)
}

// ListRepositories returns all repositories from the registry
func (s *WorkspaceService) ListRepositories() ([]domain.Repository, error) {
	return s.discovery.GetRepositories()
}

// ListRepositoriesByTags returns repositories filtered by tags
func (s *WorkspaceService) ListRepositoriesByTags(tags []string) ([]domain.Repository, error) {
	return s.discovery.GetRepositoriesByTags(tags)
}

// ListWorkspaces returns all workspaces
func (s *WorkspaceService) ListWorkspaces() ([]domain.Workspace, error) {
	configDir, err := s.deps.FS.UserConfigDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user config directory")
	}

	workspacesDir := s.deps.FS.Join(configDir, "workspace-manager", "workspaces")

	if !s.deps.FS.Exists(workspacesDir) {
		return []domain.Workspace{}, nil
	}

	entries, err := s.deps.FS.ReadDir(workspacesDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read workspaces directory")
	}

	var workspaces []domain.Workspace
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			path := s.deps.FS.Join(workspacesDir, entry.Name())
			data, err := s.deps.FS.ReadFile(path)
			if err != nil {
				s.deps.Logger.Warn("Failed to read workspace file",
					ux.Field("path", path),
					ux.Field("error", err))
				continue
			}

			var workspace domain.Workspace
			if err := json.Unmarshal(data, &workspace); err != nil {
				s.deps.Logger.Warn("Failed to parse workspace file",
					ux.Field("path", path),
					ux.Field("error", err))
				continue
			}

			workspaces = append(workspaces, workspace)
		}
	}

	return workspaces, nil
}

// GetWorkspaceStatus returns the comprehensive status of a workspace
func (s *WorkspaceService) GetWorkspaceStatus(ctx context.Context, workspace domain.Workspace) (*domain.WorkspaceStatus, error) {
	return s.status.GetWorkspaceStatus(ctx, workspace)
}

// SyncWorkspace synchronizes all repositories in a workspace
func (s *WorkspaceService) SyncWorkspace(ctx context.Context, workspace domain.Workspace, options sync.SyncOptions) ([]sync.SyncResult, error) {
	return s.sync.SyncWorkspace(ctx, workspace, options)
}

// FetchWorkspace fetches all repositories in a workspace
func (s *WorkspaceService) FetchWorkspace(ctx context.Context, workspace domain.Workspace) error {
	return s.sync.FetchWorkspace(ctx, workspace)
}

// CreateBranchWorkspace creates a branch across all repositories in a workspace
func (s *WorkspaceService) CreateBranchWorkspace(ctx context.Context, workspace domain.Workspace, branchName string, track bool) ([]sync.BranchResult, error) {
	return s.sync.CreateBranchWorkspace(ctx, workspace, branchName, track)
}

// SwitchBranchWorkspace switches to a branch across all repositories in a workspace
func (s *WorkspaceService) SwitchBranchWorkspace(ctx context.Context, workspace domain.Workspace, branchName string) ([]sync.BranchResult, error) {
	return s.sync.SwitchBranchWorkspace(ctx, workspace, branchName)
}

// ListBranchesWorkspace lists current branches across all repositories in a workspace
func (s *WorkspaceService) ListBranchesWorkspace(ctx context.Context, workspace domain.Workspace) ([]sync.BranchResult, error) {
	return s.sync.ListBranchesWorkspace(ctx, workspace)
}

// AddRepositoriesToWorkspaceRequest contains parameters for adding repositories to an existing workspace
type AddRepositoriesToWorkspaceRequest struct {
	WorkspaceName string
	RepoNames     []string
	Branch        string
	Force         bool
	DryRun        bool
}

// AddRepositoriesToWorkspace adds repositories to an existing workspace
func (s *WorkspaceService) AddRepositoriesToWorkspace(ctx context.Context, req AddRepositoriesToWorkspaceRequest) (*domain.Workspace, error) {
	s.deps.Logger.Info("Adding repositories to workspace",
		ux.Field("workspace", req.WorkspaceName),
		ux.Field("repos", req.RepoNames),
		ux.Field("branch", req.Branch),
		ux.Field("force", req.Force),
		ux.Field("dryRun", req.DryRun))

	// Load existing workspace
	workspace, err := s.LoadWorkspace(req.WorkspaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "workspace '%s' not found", req.WorkspaceName)
	}

	// Check for duplicate repositories
	for _, repoName := range req.RepoNames {
		for _, existingRepo := range workspace.Repositories {
			if existingRepo.Name == repoName {
				return nil, errors.Errorf("repository '%s' is already in workspace '%s'", repoName, req.WorkspaceName)
			}
		}
	}

	// Find repositories using discovery service
	newRepos, err := s.discovery.FindRepositories(req.RepoNames)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find repositories")
	}

	// Use workspace's branch if no specific branch provided
	targetBranch := req.Branch
	if targetBranch == "" {
		targetBranch = workspace.Branch
	}

	// Create a copy of the workspace with the new repositories
	updatedWorkspace := *workspace
	updatedWorkspace.Repositories = append(workspace.Repositories, newRepos...)
	updatedWorkspace.GoWorkspace = s.shouldCreateGoWorkspace(updatedWorkspace.Repositories)

	if req.DryRun {
		s.deps.Logger.Info("Dry run mode - skipping actual addition",
			ux.Field("workspace", req.WorkspaceName),
			ux.Field("newRepos", len(newRepos)))
		return &updatedWorkspace, nil
	}

	// Create worktrees for new repositories
	for _, repo := range newRepos {
		targetPath := workspace.RepositoryWorktreePath(repo.Name)

		if err := s.worktree.Create(ctx, repo, targetPath, targetBranch, worktree.CreateOpts{
			Force: req.Force,
		}); err != nil {
			return nil, errors.Wrapf(err, "failed to create worktree for %s", repo.Name)
		}

		s.deps.Logger.Info("Created worktree",
			ux.Field("repo", repo.Name),
			ux.Field("target", targetPath),
			ux.Field("branch", targetBranch))
	}

	// Update go.work file if needed
	if updatedWorkspace.GoWorkspace {
		content := s.gowork.Generate(updatedWorkspace)
		if err := s.deps.FS.WriteFile(updatedWorkspace.GoWorkPath(), []byte(content), 0644); err != nil {
			return nil, errors.Wrap(err, "failed to update go.work file")
		}
		s.deps.Logger.Debug("Updated go.work file", ux.Field("path", updatedWorkspace.GoWorkPath()))
	}

	// Update workspace metadata
	metadataBytes, err := s.metadata.BuildWorkspaceMetadata(updatedWorkspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build updated metadata")
	}

	if err := s.deps.FS.WriteFile(updatedWorkspace.MetadataPath(), metadataBytes, 0644); err != nil {
		return nil, errors.Wrap(err, "failed to write updated metadata")
	}
	s.deps.Logger.Debug("Updated workspace metadata", ux.Field("path", updatedWorkspace.MetadataPath()))

	// Save updated workspace configuration
	if err := s.config.SaveWorkspace(&updatedWorkspace); err != nil {
		return nil, errors.Wrap(err, "failed to save updated workspace")
	}

	s.deps.Logger.Info("Successfully added repositories to workspace",
		ux.Field("workspace", req.WorkspaceName),
		ux.Field("addedRepos", len(newRepos)))

	return &updatedWorkspace, nil
}

// DeleteWorkspace deletes a workspace and optionally removes its files
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, name string, removeFiles bool, forceWorktrees bool) error {
	s.deps.Logger.Info("Deleting workspace",
		ux.Field("name", name),
		ux.Field("removeFiles", removeFiles),
		ux.Field("forceWorktrees", forceWorktrees))

	// Load workspace to get its path
	workspace, err := s.LoadWorkspace(name)
	if err != nil {
		return errors.Wrapf(err, "workspace '%s' not found", name)
	}

	// Remove worktrees first
	if err := s.removeWorktrees(ctx, workspace, forceWorktrees); err != nil {
		return errors.Wrap(err, "failed to remove worktrees")
	}

	// Remove workspace directory and files if requested
	if removeFiles {
		if s.deps.FS.Exists(workspace.Path) {
			s.deps.Logger.Info("Removing workspace directory", ux.Field("path", workspace.Path))

			if err := s.deps.FS.RemoveAll(workspace.Path); err != nil {
				return errors.Wrapf(err, "failed to remove workspace directory: %s", workspace.Path)
			}
		}
	} else {
		// Clean up workspace-specific files (go.work, AGENT.md)
		s.cleanupWorkspaceSpecificFiles(workspace.Path)
	}

	// Remove workspace configuration
	configDir, err := s.deps.FS.UserConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get config directory")
	}

	configPath := s.deps.FS.Join(configDir, "workspace-manager", "workspaces", name+".json")
	if s.deps.FS.Exists(configPath) {
		if err := s.deps.FS.RemoveAll(configPath); err != nil {
			return errors.Wrapf(err, "failed to remove workspace configuration: %s", configPath)
		}
	}

	s.deps.Logger.Info("Workspace deleted successfully", ux.Field("name", name))
	return nil
}

// LoadWorkspace loads a specific workspace by name
func (s *WorkspaceService) LoadWorkspace(name string) (*domain.Workspace, error) {
	configDir, err := s.deps.FS.UserConfigDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get config directory")
	}

	configPath := s.deps.FS.Join(configDir, "workspace-manager", "workspaces", name+".json")

	if !s.deps.FS.Exists(configPath) {
		return nil, errors.Errorf("workspace '%s' not found", name)
	}

	data, err := s.deps.FS.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read workspace configuration: %s", configPath)
	}

	var workspace domain.Workspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return nil, errors.Wrapf(err, "failed to parse workspace configuration: %s", configPath)
	}

	return &workspace, nil
}

// DetectWorkspace detects the workspace name from the current directory
func (s *WorkspaceService) DetectWorkspace(cwd string) (string, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return "", errors.Wrap(err, "failed to load workspaces")
	}

	// Check if current directory is within any workspace path
	for _, workspace := range workspaces {
		// Check if current directory is within or matches workspace path
		if strings.HasPrefix(cwd, workspace.Path) {
			s.deps.Logger.Debug("Detected workspace from path",
				ux.Field("name", workspace.Name),
				ux.Field("path", workspace.Path),
				ux.Field("cwd", cwd))
			return workspace.Name, nil
		}

		// Also check if any repository in the workspace matches current directory
		for _, repo := range workspace.Repositories {
			repoWorktreePath := s.deps.FS.Join(workspace.Path, repo.Name)
			if strings.HasPrefix(cwd, repoWorktreePath) {
				s.deps.Logger.Debug("Detected workspace from repo path",
					ux.Field("name", workspace.Name),
					ux.Field("repo", repo.Name),
					ux.Field("repoPath", repoWorktreePath),
					ux.Field("cwd", cwd))
				return workspace.Name, nil
			}
		}
	}

	return "", errors.New("no workspace found containing current directory")
}

// removeWorktrees removes git worktrees for a workspace
func (s *WorkspaceService) removeWorktrees(ctx context.Context, workspace *domain.Workspace, force bool) error {
	var errs []error

	for _, repo := range workspace.Repositories {
		worktreePath := s.deps.FS.Join(workspace.Path, repo.Name)

		s.deps.Logger.Info("Removing worktree",
			ux.Field("repo", repo.Name),
			ux.Field("path", worktreePath))

		if !s.deps.FS.Exists(worktreePath) {
			s.deps.Logger.Debug("Worktree directory does not exist, skipping", ux.Field("path", worktreePath))
			continue
		}

		// Remove the worktree using git
		if err := s.deps.Git.WorktreeRemove(ctx, repo.Path, worktreePath, force); err != nil {
			s.deps.Logger.Error("Failed to remove worktree",
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
			errs = append(errs, errors.Wrapf(err, "failed to remove worktree for %s", repo.Name))
			continue
		}
	}

	if len(errs) > 0 {
		return errors.Errorf("failed to remove %d worktrees: %v", len(errs), errs)
	}

	return nil
}

// cleanupWorkspaceSpecificFiles removes workspace-specific files
func (s *WorkspaceService) cleanupWorkspaceSpecificFiles(workspacePath string) {
	filesToClean := []string{"go.work", "go.work.sum", "AGENT.md"}

	for _, file := range filesToClean {
		filePath := s.deps.FS.Join(workspacePath, file)
		if s.deps.FS.Exists(filePath) {
			if err := s.deps.FS.RemoveAll(filePath); err != nil {
				s.deps.Logger.Warn("Failed to remove workspace file",
					ux.Field("file", filePath),
					ux.Field("error", err))
			} else {
				s.deps.Logger.Debug("Removed workspace file", ux.Field("file", filePath))
			}
		}
	}
}

// RemoveRepositoriesFromWorkspaceRequest contains parameters for removing repositories from a workspace
type RemoveRepositoriesFromWorkspaceRequest struct {
	WorkspaceName string
	RepoNames     []string
	Force         bool
	RemoveFiles   bool
	DryRun        bool
}

// RemoveRepositoriesFromWorkspace removes repositories from an existing workspace
func (s *WorkspaceService) RemoveRepositoriesFromWorkspace(ctx context.Context, req RemoveRepositoriesFromWorkspaceRequest) (*domain.Workspace, error) {
	s.deps.Logger.Info("Removing repositories from workspace",
		ux.Field("workspace", req.WorkspaceName),
		ux.Field("repos", req.RepoNames),
		ux.Field("force", req.Force),
		ux.Field("removeFiles", req.RemoveFiles),
		ux.Field("dryRun", req.DryRun))

	// Load existing workspace
	workspace, err := s.LoadWorkspace(req.WorkspaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "workspace '%s' not found", req.WorkspaceName)
	}

	// Find repositories to remove and validate they exist in the workspace
	var toRemove []domain.Repository
	var remainingRepos []domain.Repository

	for _, repo := range workspace.Repositories {
		shouldRemove := false
		for _, repoName := range req.RepoNames {
			if repo.Name == repoName {
				shouldRemove = true
				toRemove = append(toRemove, repo)
				break
			}
		}
		if !shouldRemove {
			remainingRepos = append(remainingRepos, repo)
		}
	}

	// Check if all requested repositories were found
	if len(toRemove) != len(req.RepoNames) {
		var notFound []string
		for _, repoName := range req.RepoNames {
			found := false
			for _, repo := range toRemove {
				if repo.Name == repoName {
					found = true
					break
				}
			}
			if !found {
				notFound = append(notFound, repoName)
			}
		}
		return nil, errors.Errorf("repositories not found in workspace '%s': %v", req.WorkspaceName, notFound)
	}

	// Create a copy of the workspace with remaining repositories
	updatedWorkspace := *workspace
	updatedWorkspace.Repositories = remainingRepos
	updatedWorkspace.GoWorkspace = s.shouldCreateGoWorkspace(remainingRepos)

	if req.DryRun {
		s.deps.Logger.Info("Dry run mode - skipping actual removal",
			ux.Field("workspace", req.WorkspaceName),
			ux.Field("toRemove", len(toRemove)))
		return &updatedWorkspace, nil
	}

	// Remove worktrees for repositories
	for _, repo := range toRemove {
		worktreePath := workspace.RepositoryWorktreePath(repo.Name)

		s.deps.Logger.Info("Removing worktree",
			ux.Field("repo", repo.Name),
			ux.Field("path", worktreePath))

		if s.deps.FS.Exists(worktreePath) {
			// Remove the worktree using git
			if err := s.deps.Git.WorktreeRemove(ctx, repo.Path, worktreePath, req.Force); err != nil {
				return nil, errors.Wrapf(err, "failed to remove worktree for %s", repo.Name)
			}
		}

		// Remove repository directory if requested
		if req.RemoveFiles && s.deps.FS.Exists(worktreePath) {
			if err := s.deps.FS.RemoveAll(worktreePath); err != nil {
				return nil, errors.Wrapf(err, "failed to remove repository directory: %s", worktreePath)
			}
			s.deps.Logger.Debug("Removed repository directory", ux.Field("path", worktreePath))
		}
	}

	// Update go.work file if needed
	if updatedWorkspace.GoWorkspace {
		content := s.gowork.Generate(updatedWorkspace)
		if err := s.deps.FS.WriteFile(updatedWorkspace.GoWorkPath(), []byte(content), 0644); err != nil {
			return nil, errors.Wrap(err, "failed to update go.work file")
		}
		s.deps.Logger.Debug("Updated go.work file", ux.Field("path", updatedWorkspace.GoWorkPath()))
	}

	// Update workspace metadata
	metadataBytes, err := s.metadata.BuildWorkspaceMetadata(updatedWorkspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build updated metadata")
	}

	if err := s.deps.FS.WriteFile(updatedWorkspace.MetadataPath(), metadataBytes, 0644); err != nil {
		return nil, errors.Wrap(err, "failed to write updated metadata")
	}
	s.deps.Logger.Debug("Updated workspace metadata", ux.Field("path", updatedWorkspace.MetadataPath()))

	// Save updated workspace configuration
	if err := s.config.SaveWorkspace(&updatedWorkspace); err != nil {
		return nil, errors.Wrap(err, "failed to save updated workspace")
	}

	s.deps.Logger.Info("Successfully removed repositories from workspace",
		ux.Field("workspace", req.WorkspaceName),
		ux.Field("removedRepos", len(toRemove)))

	return &updatedWorkspace, nil
}

// TmuxSessionRequest contains parameters for creating a tmux session
type TmuxSessionRequest struct {
	WorkspaceName string
	Profile       string
	SessionName   string
}

// TmuxConfPath represents a tmux.conf file with its working directory
type TmuxConfPath struct {
	FilePath   string
	WorkingDir string
}

// CreateTmuxSession creates or attaches to a tmux session for the workspace
func (s *WorkspaceService) CreateTmuxSession(ctx context.Context, req TmuxSessionRequest) error {
	s.deps.Logger.Info("Creating tmux session",
		ux.Field("workspace", req.WorkspaceName),
		ux.Field("profile", req.Profile))

	// Load workspace
	workspace, err := s.LoadWorkspace(req.WorkspaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to load workspace '%s'", req.WorkspaceName)
	}

	// Use workspace name as session name if not specified
	sessionName := req.SessionName
	if sessionName == "" {
		sessionName = req.WorkspaceName
	}

	// Check if tmux session already exists
	sessionExists, err := s.tmuxSessionExists(ctx, sessionName)
	if err != nil {
		return errors.Wrap(err, "failed to check tmux session")
	}

	if sessionExists {
		s.deps.Logger.Info("Attaching to existing tmux session", ux.Field("session", sessionName))
		return s.attachTmuxSession(sessionName)
	}

	s.deps.Logger.Info("Creating new tmux session", ux.Field("session", sessionName))

	// Create new session in detached mode
	if err := s.createTmuxSession(ctx, sessionName, workspace.Path); err != nil {
		return errors.Wrapf(err, "failed to create tmux session '%s'", sessionName)
	}

	// Execute tmux.conf files
	if err := s.executeTmuxConfFiles(ctx, workspace, sessionName, req.Profile); err != nil {
		s.deps.Logger.Warn("Failed to execute tmux.conf files", ux.Field("error", err))
	}

	// Attach to the session
	return s.attachTmuxSession(sessionName)
}

// GetTmuxConfPaths returns tmux configuration file paths for a workspace
func (s *WorkspaceService) GetTmuxConfPaths(workspace *domain.Workspace, profile string) []TmuxConfPath {
	if profile != "" {
		return s.getTmuxConfPathsForProfile(workspace, profile)
	}
	return s.getDefaultTmuxConfPaths(workspace)
}

// tmuxSessionExists checks if a tmux session exists
func (s *WorkspaceService) tmuxSessionExists(ctx context.Context, sessionName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", sessionName)
	return cmd.Run() == nil, nil
}

// createTmuxSession creates a new tmux session
func (s *WorkspaceService) createTmuxSession(ctx context.Context, sessionName, workspacePath string) error {
	cmd := exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", sessionName, "-c", workspacePath)
	return cmd.Run()
}

// attachTmuxSession attaches to a tmux session by replacing the current process
func (s *WorkspaceService) attachTmuxSession(sessionName string) error {
	// Find tmux binary
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return errors.Wrap(err, "tmux not found in PATH")
	}

	// Prepare arguments (tmux + provided args)
	args := []string{"tmux", "attach-session", "-t", sessionName}

	// Replace current process with tmux
	err = syscall.Exec(tmuxPath, args, os.Environ())
	if err != nil {
		return errors.Wrap(err, "failed to exec tmux")
	}

	return nil
}

// executeTmuxConfFiles executes tmux configuration files for a workspace
func (s *WorkspaceService) executeTmuxConfFiles(ctx context.Context, workspace *domain.Workspace, sessionName, profile string) error {
	confPaths := s.GetTmuxConfPaths(workspace, profile)

	if profile != "" {
		s.deps.Logger.Info("Using tmux profile", ux.Field("profile", profile))
	}

	// Execute all found tmux.conf files
	for _, confPath := range confPaths {
		if err := s.executeTmuxConfFile(ctx, confPath.FilePath, sessionName, confPath.WorkingDir); err != nil {
			s.deps.Logger.Debug("Failed to execute tmux.conf",
				ux.Field("file", confPath.FilePath),
				ux.Field("error", err))
		}
	}

	return nil
}

// getTmuxConfPathsForProfile returns profile-specific tmux.conf file paths
func (s *WorkspaceService) getTmuxConfPathsForProfile(workspace *domain.Workspace, profile string) []TmuxConfPath {
	var paths []TmuxConfPath

	// Workspace root profile tmux.conf
	rootProfileConf := s.deps.FS.Join(workspace.Path, ".wsm", "profiles", profile, "tmux.conf")
	paths = append(paths, TmuxConfPath{
		FilePath:   rootProfileConf,
		WorkingDir: workspace.Path,
	})

	// Repository profile tmux.conf files
	entries, err := s.deps.FS.ReadDir(workspace.Path)
	if err != nil {
		s.deps.Logger.Debug("Failed to read workspace directory for profile tmux.conf", ux.Field("error", err))
		return paths
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirPath := s.deps.FS.Join(workspace.Path, entry.Name())
			profileConfPath := s.deps.FS.Join(dirPath, ".wsm", "profiles", profile, "tmux.conf")

			paths = append(paths, TmuxConfPath{
				FilePath:   profileConfPath,
				WorkingDir: dirPath,
			})
		}
	}

	return paths
}

// getDefaultTmuxConfPaths returns default tmux.conf file paths
func (s *WorkspaceService) getDefaultTmuxConfPaths(workspace *domain.Workspace) []TmuxConfPath {
	var paths []TmuxConfPath

	// Workspace root tmux.conf
	rootTmuxConf := s.deps.FS.Join(workspace.Path, ".wsm", "tmux.conf")
	paths = append(paths, TmuxConfPath{
		FilePath:   rootTmuxConf,
		WorkingDir: workspace.Path,
	})

	// Repository tmux.conf files
	entries, err := s.deps.FS.ReadDir(workspace.Path)
	if err != nil {
		s.deps.Logger.Debug("Failed to read workspace directory for default tmux.conf", ux.Field("error", err))
		return paths
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirPath := s.deps.FS.Join(workspace.Path, entry.Name())
			tmuxConfPath := s.deps.FS.Join(dirPath, ".wsm", "tmux.conf")

			paths = append(paths, TmuxConfPath{
				FilePath:   tmuxConfPath,
				WorkingDir: dirPath,
			})
		}
	}

	return paths
}

// executeTmuxConfFile executes a single tmux configuration file
func (s *WorkspaceService) executeTmuxConfFile(ctx context.Context, tmuxConfPath, sessionName, workingDir string) error {
	// Check if file exists
	if !s.deps.FS.Exists(tmuxConfPath) {
		return nil // File doesn't exist, not an error
	}

	s.deps.Logger.Debug("Executing tmux.conf file",
		ux.Field("file", tmuxConfPath),
		ux.Field("session", sessionName))

	content, err := s.deps.FS.ReadFile(tmuxConfPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read tmux.conf file: %s", tmuxConfPath)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		s.deps.Logger.Debug("Executing tmux command",
			ux.Field("command", line),
			ux.Field("session", sessionName))

		// Execute tmux command in the session
		cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, line, "Enter")
		cmd.Dir = workingDir

		if err := cmd.Run(); err != nil {
			s.deps.Logger.Warn("Failed to execute tmux command",
				ux.Field("command", line),
				ux.Field("error", err))
			// Continue with other commands even if one fails
		}
	}

	return nil
}

// CommitRequest contains parameters for committing changes
type CommitRequest struct {
	Message      string
	Interactive  bool
	AddAll       bool
	Push         bool
	DryRun       bool
	Template     string
	Repositories []string // Specific repositories to commit to
}

// CommitResponse contains the result of a commit operation
type CommitResponse struct {
	CommittedRepos []string            `json:"committed_repos"`
	Changes        map[string][]string `json:"changes"`
	Errors         map[string]string   `json:"errors"`
	Message        string              `json:"message"`
}

// ForkRequest contains parameters for forking a workspace
type ForkRequest struct {
	NewWorkspaceName    string
	SourceWorkspaceName string
	Branch              string
	BranchPrefix        string
	AgentSource         string
	DryRun              bool
}

// CommitWorkspace commits changes across workspace repositories
func (s *WorkspaceService) CommitWorkspace(ctx context.Context, workspace domain.Workspace, req CommitRequest) (*CommitResponse, error) {
	s.deps.Logger.Info("Committing workspace changes",
		ux.Field("workspace", workspace.Name),
		ux.Field("message", req.Message),
		ux.Field("dry_run", req.DryRun))

	response := &CommitResponse{
		CommittedRepos: []string{},
		Changes:        make(map[string][]string),
		Errors:         make(map[string]string),
		Message:        req.Message,
	}

	// Handle commit message templates
	if req.Message == "" && req.Template != "" {
		req.Message = s.getCommitMessageFromTemplate(req.Template)
		response.Message = req.Message
	}

	if req.Message == "" && !req.Interactive {
		return nil, errors.New("commit message is required. Use message flag or interactive mode")
	}

	// Get workspace changes
	changes, err := s.getWorkspaceChanges(ctx, workspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workspace changes")
	}

	if len(changes) == 0 {
		s.deps.Logger.Info("No changes found in workspace")
		return response, nil
	}

	// Filter repositories if specified
	if len(req.Repositories) > 0 {
		filteredChanges := make(map[string][]string)
		for _, repoName := range req.Repositories {
			if repoChanges, exists := changes[repoName]; exists {
				filteredChanges[repoName] = repoChanges
			}
		}
		changes = filteredChanges
	}

	// Handle interactive mode
	if req.Interactive {
		changes, req.Message, err = s.selectChangesInteractively(changes, req.Message)
		if err != nil {
			return nil, errors.Wrap(err, "interactive selection failed")
		}
		response.Message = req.Message
	}

	// Commit changes to each repository
	for repoName, fileChanges := range changes {
		repoPath := s.deps.FS.Join(workspace.Path, repoName)

		s.deps.Logger.Debug("Processing repository",
			ux.Field("repo", repoName),
			ux.Field("changes", len(fileChanges)))

		// Check if it's a git repository
		isRepo, err := s.deps.Git.IsRepository(ctx, repoPath)
		if err != nil || !isRepo {
			s.deps.Logger.Warn("Skipping non-git directory", ux.Field("path", repoPath))
			continue
		}

		// Add files if requested
		if req.AddAll {
			if err := s.addAllFiles(ctx, repoPath); err != nil {
				response.Errors[repoName] = err.Error()
				continue
			}
		} else {
			// Add specific files
			for _, filePath := range fileChanges {
				if err := s.deps.Git.Add(ctx, repoPath, filePath); err != nil {
					s.deps.Logger.Warn("Failed to add file",
						ux.Field("repo", repoName),
						ux.Field("file", filePath),
						ux.Field("error", err))
				}
			}
		}

		// Check if there are staged changes
		hasChanges, err := s.deps.Git.HasChanges(ctx, repoPath)
		if err != nil {
			response.Errors[repoName] = err.Error()
			continue
		}

		if !hasChanges {
			s.deps.Logger.Debug("No staged changes in repository", ux.Field("repo", repoName))
			continue
		}

		response.Changes[repoName] = fileChanges

		// Perform commit
		if !req.DryRun {
			if err := s.deps.Git.Commit(ctx, repoPath, req.Message); err != nil {
				response.Errors[repoName] = err.Error()
				continue
			}

			// Push if requested
			if req.Push {
				if err := s.deps.Git.Push(ctx, repoPath, "origin", ""); err != nil {
					s.deps.Logger.Warn("Failed to push repository",
						ux.Field("repo", repoName),
						ux.Field("error", err))
				}
			}
		}

		response.CommittedRepos = append(response.CommittedRepos, repoName)
	}

	s.deps.Logger.Info("Commit operation completed",
		ux.Field("committed_repos", len(response.CommittedRepos)),
		ux.Field("errors", len(response.Errors)))

	return response, nil
}

// getWorkspaceChanges gets all changes across workspace repositories
func (s *WorkspaceService) getWorkspaceChanges(ctx context.Context, workspace domain.Workspace) (map[string][]string, error) {
	changes := make(map[string][]string)

	for _, repo := range workspace.Repositories {
		repoPath := s.deps.FS.Join(workspace.Path, repo.Name)

		// Check if it's a git repository
		isRepo, err := s.deps.Git.IsRepository(ctx, repoPath)
		if err != nil || !isRepo {
			continue
		}

		// Get repository status
		status, err := s.deps.Git.Status(ctx, repoPath)
		if err != nil {
			s.deps.Logger.Warn("Failed to get repository status",
				ux.Field("repo", repo.Name),
				ux.Field("error", err))
			continue
		}

		// Collect all changed files
		var repoChanges []string
		repoChanges = append(repoChanges, status.StagedFiles...)
		repoChanges = append(repoChanges, status.ModifiedFiles...)

		if len(repoChanges) > 0 {
			changes[repo.Name] = repoChanges
		}
	}

	return changes, nil
}

// addAllFiles adds all files in a repository
func (s *WorkspaceService) addAllFiles(ctx context.Context, repoPath string) error {
	return s.deps.Git.Add(ctx, repoPath, ".")
}

// selectChangesInteractively allows user to select files interactively
func (s *WorkspaceService) selectChangesInteractively(allChanges map[string][]string, initialMessage string) (map[string][]string, string, error) {
	// For now, return all changes - interactive selection would require more complex UI
	// This matches the existing behavior where interactive mode shows all changes but doesn't yet implement selection

	message := initialMessage
	if message == "" {
		return nil, "", errors.New("interactive mode requires commit message to be provided")
	}

	return allChanges, message, nil
}

// getCommitMessageFromTemplate gets commit message from template
func (s *WorkspaceService) getCommitMessageFromTemplate(template string) string {
	templates := map[string]string{
		"feature":  "feat: add new feature",
		"fix":      "fix: resolve issue",
		"docs":     "docs: update documentation",
		"style":    "style: formatting changes",
		"refactor": "refactor: code restructuring",
		"test":     "test: add or update tests",
		"chore":    "chore: maintenance tasks",
	}

	if msg, exists := templates[template]; exists {
		return msg
	}

	return template // Use template as-is if not found in predefined templates
}

// PushRequest contains parameters for pushing changes
type PushRequest struct {
	RemoteName   string
	DryRun       bool
	Force        bool
	SetUpstream  bool
	Repositories []string // Specific repositories to push
}

// PushResponse contains the result of a push operation
type PushResponse struct {
	PushedRepos []string                 `json:"pushed_repos"`
	Candidates  map[string]PushCandidate `json:"candidates"`
	Errors      map[string]string        `json:"errors"`
	RemoteName  string                   `json:"remote_name"`
}

// PushCandidate represents a repository branch that could be pushed
type PushCandidate struct {
	Repository         string `json:"repository"`
	Branch             string `json:"branch"`
	RepoPath           string `json:"repo_path"`
	LocalCommits       int    `json:"local_commits"`
	RemoteRepo         string `json:"remote_repo"`
	RemoteExists       bool   `json:"remote_exists"`
	RemoteBranchExists bool   `json:"remote_branch_exists"`
}

// PushWorkspace pushes changes across workspace repositories to specified remote
func (s *WorkspaceService) PushWorkspace(ctx context.Context, workspace domain.Workspace, req PushRequest) (*PushResponse, error) {
	s.deps.Logger.Info("Pushing workspace changes",
		ux.Field("workspace", workspace.Name),
		ux.Field("remote", req.RemoteName),
		ux.Field("dry_run", req.DryRun))

	response := &PushResponse{
		PushedRepos: []string{},
		Candidates:  make(map[string]PushCandidate),
		Errors:      make(map[string]string),
		RemoteName:  req.RemoteName,
	}

	// Find branches that need pushing
	candidates, err := s.findPushCandidates(ctx, workspace, req.RemoteName, req.Repositories)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find push candidates")
	}

	if len(candidates) == 0 {
		s.deps.Logger.Info("No branches found that need pushing", ux.Field("remote", req.RemoteName))
		return response, nil
	}

	response.Candidates = candidates

	if req.DryRun {
		s.deps.Logger.Info("Dry run mode - no branches will be pushed")
		return response, nil
	}

	// Push branches
	for repoName, candidate := range candidates {
		if !candidate.RemoteExists {
			response.Errors[repoName] = "remote repository not found or not accessible"
			continue
		}

		// Push the branch
		if err := s.pushRepositoryBranch(ctx, candidate, req.RemoteName, req.SetUpstream); err != nil {
			response.Errors[repoName] = err.Error()
			s.deps.Logger.Warn("Failed to push repository",
				ux.Field("repo", repoName),
				ux.Field("branch", candidate.Branch),
				ux.Field("error", err))
		} else {
			response.PushedRepos = append(response.PushedRepos, repoName)
			s.deps.Logger.Debug("Successfully pushed repository",
				ux.Field("repo", repoName),
				ux.Field("branch", candidate.Branch),
				ux.Field("remote", req.RemoteName))
		}
	}

	s.deps.Logger.Info("Push operation completed",
		ux.Field("pushed_repos", len(response.PushedRepos)),
		ux.Field("errors", len(response.Errors)))

	return response, nil
}

// findPushCandidates finds repository branches that need pushing
func (s *WorkspaceService) findPushCandidates(ctx context.Context, workspace domain.Workspace, remoteName string, repositories []string) (map[string]PushCandidate, error) {
	candidates := make(map[string]PushCandidate)

	// Get workspace status
	workspaceStatus, err := s.status.GetWorkspaceStatus(ctx, workspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workspace status")
	}

	for _, repoStatus := range workspaceStatus.Repositories {
		// Filter repositories if specified
		if len(repositories) > 0 {
			found := false
			for _, repoName := range repositories {
				if repoStatus.Repository.Name == repoName {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if candidate, needsPush := s.checkIfNeedsPush(ctx, repoStatus, workspace.Path, remoteName); needsPush {
			candidates[repoStatus.Repository.Name] = candidate
		}
	}

	return candidates, nil
}

// checkIfNeedsPush checks if a repository branch needs pushing
func (s *WorkspaceService) checkIfNeedsPush(ctx context.Context, repoStatus domain.RepositoryStatus, workspacePath, remoteName string) (PushCandidate, bool) {
	candidate := PushCandidate{
		Repository: repoStatus.Repository.Name,
		Branch:     repoStatus.CurrentBranch,
		RepoPath:   s.deps.FS.Join(workspacePath, repoStatus.Repository.Name),
	}

	s.deps.Logger.Debug("Checking if repository branch needs pushing",
		ux.Field("repository", candidate.Repository),
		ux.Field("branch", candidate.Branch),
		ux.Field("remote", remoteName))

	// Skip if no current branch
	if repoStatus.CurrentBranch == "" {
		s.deps.Logger.Debug("Skipping: no current branch", ux.Field("repository", candidate.Repository))
		return candidate, false
	}

	// Get repository info from GitHub
	repoInfo, err := s.getRepoInfo(ctx, candidate.RepoPath)
	if err != nil {
		s.deps.Logger.Debug("Failed to get repository info",
			ux.Field("repository", candidate.Repository),
			ux.Field("error", err))
		return candidate, false
	}

	candidate.RemoteRepo = repoInfo.NameWithOwner
	candidate.RemoteExists = true

	// Check if remote repository exists
	if !s.checkRemoteRepoExists(ctx, remoteName, repoInfo.NameWithOwner) {
		s.deps.Logger.Debug("Remote repository not accessible",
			ux.Field("repository", candidate.Repository),
			ux.Field("remote", remoteName),
			ux.Field("remoteRepo", repoInfo.NameWithOwner))
		candidate.RemoteExists = false
	}

	// Get local commits that aren't pushed to the remote yet
	localCommits, err := s.getLocalCommits(ctx, candidate.RepoPath, remoteName, candidate.Branch)
	if err != nil {
		s.deps.Logger.Debug("Failed to get local commits",
			ux.Field("repository", candidate.Repository),
			ux.Field("branch", candidate.Branch),
			ux.Field("error", err))
		localCommits = 1 // Assume there might be commits
	}

	candidate.LocalCommits = localCommits

	// Check if remote branch exists
	if candidate.RemoteExists {
		candidate.RemoteBranchExists = s.checkRemoteBranchExists(ctx, candidate.RepoPath, remoteName, candidate.Branch)
	}

	needsPush := localCommits > 0

	s.deps.Logger.Debug("Push candidate evaluation",
		ux.Field("repository", candidate.Repository),
		ux.Field("branch", candidate.Branch),
		ux.Field("needs_push", needsPush),
		ux.Field("local_commits", localCommits))

	return candidate, needsPush
}

// RepoInfo represents GitHub repository information
type RepoInfo struct {
	NameWithOwner    string `json:"nameWithOwner"`
	URL              string `json:"url"`
	DefaultBranchRef struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

// getRepoInfo gets repository information from GitHub
func (s *WorkspaceService) getRepoInfo(ctx context.Context, repoPath string) (*RepoInfo, error) {
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner,url,defaultBranchRef")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get repository info from GitHub")
	}

	var info RepoInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, errors.Wrap(err, "failed to parse repository info")
	}

	s.deps.Logger.Debug("Got repository info",
		ux.Field("repoPath", repoPath),
		ux.Field("nameWithOwner", info.NameWithOwner))

	return &info, nil
}

// checkRemoteRepoExists checks if remote repository exists and is accessible
func (s *WorkspaceService) checkRemoteRepoExists(ctx context.Context, remoteName, repoFullName string) bool {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		s.deps.Logger.Debug("Invalid repository name format", ux.Field("repoFullName", repoFullName))
		return false
	}

	remoteRepo := remoteName + "/" + parts[1]
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", remoteRepo)
	err := cmd.Run()

	exists := err == nil
	s.deps.Logger.Debug("Checked remote repository existence",
		ux.Field("remoteName", remoteName),
		ux.Field("repoFullName", repoFullName),
		ux.Field("remoteRepo", remoteRepo),
		ux.Field("exists", exists))

	return exists
}

// getLocalCommits gets count of local commits not pushed to remote
func (s *WorkspaceService) getLocalCommits(ctx context.Context, repoPath, remoteName, branch string) (int, error) {
	remoteRef := remoteName + "/" + branch

	// Try to get commits ahead of remote branch
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", remoteRef+"..HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()

	if err != nil {
		// Remote branch might not exist, check against origin/main
		s.deps.Logger.Debug("Remote branch not found, checking against origin/main",
			ux.Field("repoPath", repoPath),
			ux.Field("remoteRef", remoteRef),
			ux.Field("error", err))

		cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", "origin/main..HEAD")
		cmd.Dir = repoPath
		output, err = cmd.Output()
		if err != nil {
			// Fallback: count commits on current branch
			cmd = exec.CommandContext(ctx, "git", "rev-list", "--count", "HEAD")
			cmd.Dir = repoPath
			output, err = cmd.Output()
			if err != nil {
				return 0, err
			}
		}
	}

	count := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
		return 0, err
	}

	s.deps.Logger.Debug("Got local commits count",
		ux.Field("repoPath", repoPath),
		ux.Field("remoteName", remoteName),
		ux.Field("branch", branch),
		ux.Field("localCommits", count))

	return count, nil
}

// checkRemoteBranchExists checks if a branch exists on the remote
func (s *WorkspaceService) checkRemoteBranchExists(ctx context.Context, repoPath, remoteName, branch string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", remoteName, branch)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(output))) > 0
}

// pushRepositoryBranch pushes a branch to the remote repository
func (s *WorkspaceService) pushRepositoryBranch(ctx context.Context, candidate PushCandidate, remoteName string, setUpstream bool) error {
	args := []string{"push"}

	if setUpstream {
		args = append(args, "-u")
	}

	args = append(args, remoteName, candidate.Branch)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = candidate.RepoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "git push failed: %s", string(output))
	}

	s.deps.Logger.Debug("Successfully pushed branch",
		ux.Field("repository", candidate.Repository),
		ux.Field("branch", candidate.Branch),
		ux.Field("remote", remoteName))

	return nil
}

// ForkWorkspace creates a new workspace by forking an existing workspace
func (s *WorkspaceService) ForkWorkspace(ctx context.Context, req ForkRequest) (*domain.Workspace, error) {
	s.deps.Logger.Info("Forking workspace",
		ux.Field("source", req.SourceWorkspaceName),
		ux.Field("new", req.NewWorkspaceName))

	// Load source workspace
	sourceWorkspace, err := s.LoadWorkspace(req.SourceWorkspaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load source workspace '%s'", req.SourceWorkspaceName)
	}

	// Get current branch status of source workspace to use as base branch
	status, err := s.status.GetWorkspaceStatus(ctx, *sourceWorkspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get source workspace status")
	}

	// Determine the base branch from the source workspace
	// Use the first repository's current branch as the base
	var baseBranch string
	if len(status.Repositories) > 0 {
		baseBranch = status.Repositories[0].CurrentBranch
		s.deps.Logger.Info("Using base branch", ux.Field("branch", baseBranch))
	}

	// Validate that all repositories are on the same branch
	for _, repoStatus := range status.Repositories {
		if repoStatus.CurrentBranch != baseBranch {
			return nil, errors.Errorf("repositories in source workspace are on different branches: %s is on %s, but expected %s",
				repoStatus.Repository.Name, repoStatus.CurrentBranch, baseBranch)
		}
	}

	// Generate branch name if not specified
	finalBranch := req.Branch
	if finalBranch == "" {
		finalBranch = fmt.Sprintf("%s/%s", req.BranchPrefix, req.NewWorkspaceName)
		s.deps.Logger.Info("Using auto-generated branch", ux.Field("branch", finalBranch))
	}

	// Extract repository names from source workspace
	var repoNames []string
	for _, repo := range sourceWorkspace.Repositories {
		repoNames = append(repoNames, repo.Name)
	}

	// Load AGENT.md content if specified
	var agentMDContent string
	if req.AgentSource != "" {
		content, err := s.deps.FS.ReadFile(req.AgentSource)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read agent source file: %s", req.AgentSource)
		}
		agentMDContent = string(content)
	} else if sourceWorkspace.AgentMD != "" {
		// Use the source workspace's agent MD if no custom one specified
		agentMDContent = sourceWorkspace.AgentMD
		s.deps.Logger.Info("Using AGENT.md from source workspace")
	}

	// Create the new workspace using the service
	workspace, err := s.Create(ctx, CreateRequest{
		Name:       req.NewWorkspaceName,
		RepoNames:  repoNames,
		Branch:     finalBranch,
		BaseBranch: baseBranch,
		AgentMD:    agentMDContent,
		DryRun:     req.DryRun,
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to fork workspace")
	}

	s.deps.Logger.Info("Workspace forked successfully",
		ux.Field("source", req.SourceWorkspaceName),
		ux.Field("new", req.NewWorkspaceName),
		ux.Field("branch", finalBranch),
		ux.Field("baseBranch", baseBranch))

	return workspace, nil
}

// MergeRequest contains parameters for merging a workspace
type MergeRequest struct {
	WorkspaceName string
	DryRun        bool
	Force         bool
	KeepWorkspace bool
}

// MergeCandidate represents a repository that can be merged
type MergeCandidate struct {
	Repository    domain.Repository
	WorktreePath  string
	BaseBranch    string
	CurrentBranch string
	HasChanges    bool
	IsClean       bool
}

// MergeResponse contains the result of a merge operation
type MergeResponse struct {
	WorkspaceName    string            `json:"workspace_name"`
	BaseBranch       string            `json:"base_branch"`
	CurrentBranch    string            `json:"current_branch"`
	MergedRepos      []string          `json:"merged_repos"`
	Candidates       []MergeCandidate  `json:"candidates"`
	Errors           map[string]string `json:"errors"`
	WorkspaceDeleted bool              `json:"workspace_deleted"`
}

// MergeWorkspace merges a forked workspace back into its base branch
func (s *WorkspaceService) MergeWorkspace(ctx context.Context, req MergeRequest) (*MergeResponse, error) {
	s.deps.Logger.Info("Starting merge operation",
		ux.Field("workspace", req.WorkspaceName),
		ux.Field("dryRun", req.DryRun),
		ux.Field("force", req.Force),
		ux.Field("keepWorkspace", req.KeepWorkspace))

	// Load workspace
	workspace, err := s.LoadWorkspace(req.WorkspaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load workspace '%s'", req.WorkspaceName)
	}

	// Verify this is a forked workspace
	if workspace.BaseBranch == "" {
		return nil, errors.New("workspace is not a fork (no base branch specified). Only forked workspaces can be merged")
	}

	// Check if there's a workspace for the base branch and validate location
	if err := s.validateMergeLocation(workspace); err != nil {
		return nil, err
	}

	// Get workspace status to prepare merge candidates
	status, err := s.status.GetWorkspaceStatus(ctx, *workspace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get workspace status")
	}

	// Prepare merge candidates
	candidates, uncleanRepos, err := s.prepareMergeCandidates(workspace, status)
	if err != nil {
		return nil, err
	}

	// Check for unclean repositories
	if len(uncleanRepos) > 0 && !req.Force {
		return nil, errors.Errorf("the following repositories have uncommitted changes: %s. Commit or stash changes first, or use --force",
			strings.Join(uncleanRepos, ", "))
	}

	// Verify all repositories are on the workspace branch
	if err := s.validateRepositoryBranches(workspace, candidates); err != nil {
		return nil, err
	}

	response := &MergeResponse{
		WorkspaceName: workspace.Name,
		BaseBranch:    workspace.BaseBranch,
		CurrentBranch: workspace.Branch,
		Candidates:    candidates,
		Errors:        make(map[string]string),
	}

	if req.DryRun {
		s.deps.Logger.Info("Dry run mode - merge would be successful", ux.Field("candidates", len(candidates)))
		return response, nil
	}

	// Execute merge
	return s.executeMerge(ctx, workspace, candidates, req.KeepWorkspace, response)
}

// validateMergeLocation checks if merge should be run from the base workspace
func (s *WorkspaceService) validateMergeLocation(workspace *domain.Workspace) error {
	// Find workspace for the base branch
	baseWorkspace, err := s.findWorkspaceByBranch(workspace.BaseBranch)
	if err != nil {
		return errors.Wrap(err, "failed to check for base branch workspace")
	}

	// If there's a workspace for the base branch, ensure we're running from within it
	if baseWorkspace != nil {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get current directory")
		}

		// Check if current directory is within the base workspace
		if !strings.HasPrefix(cwd, baseWorkspace.Path) {
			return errors.Errorf("found workspace '%s' for base branch '%s'. Please run the merge command from within that workspace (at %s) to avoid git worktree conflicts",
				baseWorkspace.Name, workspace.BaseBranch, baseWorkspace.Path)
		}

		s.deps.Logger.Info("Running merge from base workspace as required", ux.Field("baseWorkspace", baseWorkspace.Name))
	}

	return nil
}

// prepareMergeCandidates prepares the list of repositories to merge
func (s *WorkspaceService) prepareMergeCandidates(workspace *domain.Workspace, status *domain.WorkspaceStatus) ([]MergeCandidate, []string, error) {
	var candidates []MergeCandidate
	var uncleanRepos []string

	for _, repoStatus := range status.Repositories {
		candidate := MergeCandidate{
			Repository:    repoStatus.Repository,
			WorktreePath:  s.deps.FS.Join(workspace.Path, repoStatus.Repository.Name),
			BaseBranch:    workspace.BaseBranch,
			CurrentBranch: repoStatus.CurrentBranch,
			HasChanges:    repoStatus.HasChanges,
			IsClean:       !repoStatus.HasChanges && len(repoStatus.StagedFiles) == 0 && len(repoStatus.UntrackedFiles) == 0,
		}

		candidates = append(candidates, candidate)

		if !candidate.IsClean {
			uncleanRepos = append(uncleanRepos, repoStatus.Repository.Name)
		}
	}

	return candidates, uncleanRepos, nil
}

// validateRepositoryBranches ensures all repositories are on the workspace branch
func (s *WorkspaceService) validateRepositoryBranches(workspace *domain.Workspace, candidates []MergeCandidate) error {
	for _, candidate := range candidates {
		if candidate.CurrentBranch != workspace.Branch {
			return errors.Errorf("repository '%s' is on branch '%s', expected '%s'. Switch all repositories to the workspace branch first",
				candidate.Repository.Name, candidate.CurrentBranch, workspace.Branch)
		}
	}
	return nil
}

// executeMerge performs the actual merge operation
func (s *WorkspaceService) executeMerge(ctx context.Context, workspace *domain.Workspace, candidates []MergeCandidate, keepWorkspace bool, response *MergeResponse) (*MergeResponse, error) {
	var successfulMerges []string

	// Execute merge for each repository
	for _, candidate := range candidates {
		s.deps.Logger.Info("Processing repository", ux.Field("repo", candidate.Repository.Name))

		if err := s.mergeRepository(ctx, candidate); err != nil {
			s.deps.Logger.Error("Failed to merge repository",
				ux.Field("repo", candidate.Repository.Name),
				ux.Field("error", err))

			response.Errors[candidate.Repository.Name] = err.Error()

			// Rollback successful merges
			if len(successfulMerges) > 0 {
				s.deps.Logger.Warn("Rolling back successful merges due to failure...")
				s.rollbackMerges(ctx, workspace, successfulMerges)
			}

			return response, errors.Wrapf(err, "merge failed for repository %s", candidate.Repository.Name)
		}

		successfulMerges = append(successfulMerges, candidate.Repository.Name)
		s.deps.Logger.Info("Successfully merged repository", ux.Field("repo", candidate.Repository.Name))
	}

	response.MergedRepos = successfulMerges

	// Delete workspace if requested
	if !keepWorkspace {
		s.deps.Logger.Info("Deleting workspace", ux.Field("workspace", workspace.Name))

		if err := s.DeleteWorkspace(ctx, workspace.Name, true, true); err != nil {
			s.deps.Logger.Warn("Failed to delete workspace", ux.Field("error", err))
		} else {
			response.WorkspaceDeleted = true
			s.deps.Logger.Info("Workspace deleted successfully", ux.Field("workspace", workspace.Name))
		}
	}

	s.deps.Logger.Info("Merge completed successfully",
		ux.Field("workspace", workspace.Name),
		ux.Field("mergedRepos", len(successfulMerges)))

	return response, nil
}

// mergeRepository merges a single repository
func (s *WorkspaceService) mergeRepository(ctx context.Context, candidate MergeCandidate) error {
	repoPath := candidate.WorktreePath

	s.deps.Logger.Debug("Starting repository merge",
		ux.Field("repository", candidate.Repository.Name),
		ux.Field("repoPath", repoPath),
		ux.Field("currentBranch", candidate.CurrentBranch),
		ux.Field("baseBranch", candidate.BaseBranch))

	// Step 1: Fetch latest changes
	if err := s.deps.Git.Fetch(ctx, repoPath, "origin"); err != nil {
		return errors.Wrap(err, "failed to fetch latest changes")
	}

	// Step 2: Switch to base branch
	if err := s.deps.Git.Checkout(ctx, repoPath, candidate.BaseBranch); err != nil {
		return errors.Wrapf(err, "failed to switch to base branch %s", candidate.BaseBranch)
	}

	// Step 3: Pull latest base branch changes
	if err := s.deps.Git.Pull(ctx, repoPath, "origin", candidate.BaseBranch); err != nil {
		return errors.Wrapf(err, "failed to pull latest changes for %s", candidate.BaseBranch)
	}

	// Step 4: Merge workspace branch
	if err := s.deps.Git.Merge(ctx, repoPath, candidate.CurrentBranch); err != nil {
		// Check if this is a merge conflict
		if s.isGitMergeConflict(err) {
			return errors.Errorf("merge conflict detected in %s. Please resolve conflicts manually and retry", candidate.Repository.Name)
		}
		return errors.Wrapf(err, "failed to merge %s into %s", candidate.CurrentBranch, candidate.BaseBranch)
	}

	// Step 5: Push merged changes
	if err := s.deps.Git.Push(ctx, repoPath, "origin", candidate.BaseBranch); err != nil {
		return errors.Wrapf(err, "failed to push merged changes for %s", candidate.BaseBranch)
	}

	s.deps.Logger.Debug("Repository merge completed successfully", ux.Field("repository", candidate.Repository.Name))
	return nil
}

// isGitMergeConflict checks if an error is a git merge conflict
func (s *WorkspaceService) isGitMergeConflict(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "conflict") ||
		strings.Contains(errStr, "merge failed") ||
		strings.Contains(errStr, "automatic merge failed")
}

// rollbackMerges rolls back successful merges
func (s *WorkspaceService) rollbackMerges(ctx context.Context, workspace *domain.Workspace, successfulMerges []string) {
	s.deps.Logger.Warn("Rolling back successful merges", ux.Field("count", len(successfulMerges)))

	for _, repoName := range successfulMerges {
		repoPath := s.deps.FS.Join(workspace.Path, repoName)

		s.deps.Logger.Info("Rolling back repository", ux.Field("repo", repoName))

		// Reset base branch to origin state
		if err := s.deps.Git.Checkout(ctx, repoPath, workspace.BaseBranch); err != nil {
			s.deps.Logger.Warn("Failed to checkout base branch during rollback",
				ux.Field("repo", repoName),
				ux.Field("error", err))
			continue
		}

		if err := s.deps.Git.ResetHard(ctx, repoPath, "origin/"+workspace.BaseBranch); err != nil {
			s.deps.Logger.Warn("Failed to reset base branch during rollback",
				ux.Field("repo", repoName),
				ux.Field("error", err))
			continue
		}

		// Switch back to workspace branch
		if err := s.deps.Git.Checkout(ctx, repoPath, workspace.Branch); err != nil {
			s.deps.Logger.Warn("Failed to checkout workspace branch during rollback",
				ux.Field("repo", repoName),
				ux.Field("error", err))
		}

		s.deps.Logger.Info("Rolled back repository", ux.Field("repo", repoName))
	}

	s.deps.Logger.Info("Rollback completed")
}

// findWorkspaceByBranch finds a workspace that uses the given branch
func (s *WorkspaceService) findWorkspaceByBranch(branchName string) (*domain.Workspace, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load workspaces")
	}

	for _, workspace := range workspaces {
		if workspace.Branch == branchName {
			return &workspace, nil
		}
	}

	return nil, nil // No workspace found for this branch
}

// DiffRequest contains parameters for getting workspace diff
type DiffRequest struct {
	Workspace  domain.Workspace
	Staged     bool
	RepoFilter string
}

// GetWorkspaceDiff returns unified diff across workspace repositories
func (s *WorkspaceService) GetWorkspaceDiff(ctx context.Context, req DiffRequest) (string, error) {
	s.deps.Logger.Info("Getting workspace diff",
		ux.Field("workspace", req.Workspace.Name),
		ux.Field("staged", req.Staged),
		ux.Field("repo_filter", req.RepoFilter))

	var allDiffs []string

	for _, repo := range req.Workspace.Repositories {
		if req.RepoFilter != "" && repo.Name != req.RepoFilter {
			continue
		}

		repoPath := s.deps.FS.Join(req.Workspace.Path, repo.Name)

		// Check if repository directory exists
		if !s.deps.FS.Exists(repoPath) {
			s.deps.Logger.Warn("Repository directory not found",
				ux.Field("repository", repo.Name),
				ux.Field("path", repoPath))
			continue
		}

		diff, err := s.getRepositoryDiff(ctx, repo.Name, repoPath, req.Staged)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get diff for %s", repo.Name)
		}

		if diff != "" {
			header := fmt.Sprintf("=== Repository: %s ===", repo.Name)
			allDiffs = append(allDiffs, header, diff)
		}
	}

	if len(allDiffs) == 0 {
		return "No changes found in workspace.", nil
	}

	return strings.Join(allDiffs, "\n"), nil
}

// getRepositoryDiff gets diff for a single repository
func (s *WorkspaceService) getRepositoryDiff(ctx context.Context, repoName, repoPath string, staged bool) (string, error) {
	var cmd *exec.Cmd
	if staged {
		cmd = exec.CommandContext(ctx, "git", "diff", "--cached")
	} else {
		cmd = exec.CommandContext(ctx, "git", "diff")
	}
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get diff for %s", repoName)
	}

	return strings.TrimSpace(string(output)), nil
}

// StarshipRequest contains parameters for generating starship configuration
type StarshipRequest struct {
	Symbol   string
	Style    string
	ShowDate bool
	Force    bool
}

// StarshipResponse contains the generated configuration and file path
type StarshipResponse struct {
	Config     string
	ConfigPath string
}

// GenerateStarshipConfig generates starship prompt configuration for workspaces
func (s *WorkspaceService) GenerateStarshipConfig(ctx context.Context, req StarshipRequest) (*StarshipResponse, error) {
	s.deps.Logger.Info("Generating starship configuration",
		ux.Field("symbol", req.Symbol),
		ux.Field("style", req.Style),
		ux.Field("show_date", req.ShowDate))

	config := s.generateStarshipConfig(req.Symbol, req.Style, req.ShowDate)
	configPath, err := s.getStarshipConfigPath()
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine starship config path")
	}

	return &StarshipResponse{
		Config:     config,
		ConfigPath: configPath,
	}, nil
}

// ApplyStarshipConfig appends the starship configuration to the config file
func (s *WorkspaceService) ApplyStarshipConfig(ctx context.Context, configPath, config string) error {
	s.deps.Logger.Info("Applying starship configuration", ux.Field("config_path", configPath))

	// Create the config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := s.deps.FS.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Check if file exists and get its size
	var needsNewline bool
	if s.deps.FS.Exists(configPath) {
		info, err := s.deps.FS.Stat(configPath)
		if err != nil {
			return errors.Wrap(err, "failed to get file stats")
		}
		needsNewline = info.Size() > 0
	}

	// Prepare content to write
	content := config + "\n"
	if needsNewline {
		content = "\n\n" + content
	}

	// Append to file using os.OpenFile directly since FS interface doesn't support it
	file, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to open config file")
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()

	if _, err := file.WriteString(content); err != nil {
		return errors.Wrap(err, "failed to write configuration")
	}

	return nil
}

// generateStarshipConfig creates the starship configuration string
func (s *WorkspaceService) generateStarshipConfig(symbol, style string, showDate bool) string {
	var command string
	if showDate {
		command = `printf "%s\n" "$PWD" \
  | sed -E 's|.*/workspaces/([0-9]{4}-[0-9]{2}-[0-9]{2})/([^/]+).*|\2 (\1)|'`
	} else {
		command = `printf "%s\n" "$PWD" \
  | sed -E 's|.*/workspaces/[0-9]{4}-[0-9]{2}-[0-9]{2}/([^/]+).*|\1|'`
	}

	return fmt.Sprintf(`[custom.workspace]
description = "Show current workspaces/YYYY-MM-DD/<name>"
when   = 'echo "$PWD" | grep -Eq "/workspaces/[0-9]{4}-[0-9]{2}-[0-9]{2}/"'
command = '''
  %s
'''
symbol  = "%s"
style   = "%s"
format  = '[ $symbol$output ]($style)'`, command, symbol, style)
}

// getStarshipConfigPath determines the correct starship config file path
func (s *WorkspaceService) getStarshipConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Check for XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "starship.toml"), nil
	}

	// Default to ~/.config/starship.toml
	return filepath.Join(homeDir, ".config", "starship.toml"), nil
}

// RebaseRequest contains options for rebasing workspace repositories
type RebaseRequest struct {
	TargetBranch string
	Repository   string
	Interactive  bool
	DryRun       bool
}

// RebaseResult represents the result of a rebase operation
type RebaseResult struct {
	Repository    string `json:"repository"`
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
	Rebased       bool   `json:"rebased"`
	Conflicts     bool   `json:"conflicts"`
	CommitsBefore int    `json:"commits_before"`
	CommitsAfter  int    `json:"commits_after"`
	TargetBranch  string `json:"target_branch"`
	CurrentBranch string `json:"current_branch"`
}

// RebaseResponse contains the results of a workspace rebase operation
type RebaseResponse struct {
	Results       []RebaseResult `json:"results"`
	SuccessCount  int            `json:"success_count"`
	ErrorCount    int            `json:"error_count"`
	ConflictCount int            `json:"conflict_count"`
}

// RebaseWorkspace rebases repositories in the workspace against a target branch
func (ws *WorkspaceService) RebaseWorkspace(ctx context.Context, workspace domain.Workspace, request RebaseRequest) (*RebaseResponse, error) {
	ws.deps.Logger.Info("Starting workspace rebase",
		ux.Field("workspace", workspace.Name),
		ux.Field("target_branch", request.TargetBranch),
		ux.Field("repository", request.Repository),
		ux.Field("interactive", request.Interactive),
		ux.Field("dry_run", request.DryRun))

	var results []RebaseResult

	if request.Repository != "" {
		// Rebase specific repository
		result, err := ws.rebaseRepository(ctx, workspace, request.Repository, request)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to rebase repository '%s'", request.Repository)
		}
		results = append(results, *result)
	} else {
		// Rebase all repositories
		for _, repo := range workspace.Repositories {
			result, err := ws.rebaseRepository(ctx, workspace, repo.Name, request)
			if err != nil {
				ws.deps.Logger.Error("Failed to rebase repository",
					ux.Field("repository", repo.Name),
					ux.Field("error", err))
				// Continue with other repositories
				results = append(results, RebaseResult{
					Repository: repo.Name,
					Success:    false,
					Error:      err.Error(),
				})
				continue
			}
			results = append(results, *result)
		}
	}

	// Count results
	successCount := 0
	errorCount := 0
	conflictCount := 0

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			errorCount++
		}
		if result.Conflicts {
			conflictCount++
		}
	}

	response := &RebaseResponse{
		Results:       results,
		SuccessCount:  successCount,
		ErrorCount:    errorCount,
		ConflictCount: conflictCount,
	}

	ws.deps.Logger.Info("Workspace rebase completed",
		ux.Field("success_count", successCount),
		ux.Field("error_count", errorCount),
		ux.Field("conflict_count", conflictCount))

	return response, nil
}

// rebaseRepository performs a rebase operation on a single repository
func (ws *WorkspaceService) rebaseRepository(ctx context.Context, workspace domain.Workspace, repoName string, request RebaseRequest) (*RebaseResult, error) {
	result := &RebaseResult{
		Repository:   repoName,
		Success:      true,
		TargetBranch: request.TargetBranch,
	}

	repoPath := filepath.Join(workspace.Path, repoName)

	// Check if repository exists
	isRepo, err := ws.deps.Git.IsRepository(ctx, repoPath)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to check if path is repository: %v", err)
		return result, nil
	}
	if !isRepo {
		result.Success = false
		result.Error = "not a git repository"
		return result, nil
	}

	// Get current branch
	currentBranch, err := ws.deps.Git.CurrentBranch(ctx, repoPath)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to get current branch: %v", err)
		return result, nil
	}
	result.CurrentBranch = currentBranch

	// Check if we're already on the target branch
	if currentBranch == request.TargetBranch {
		result.Success = true
		result.Error = fmt.Sprintf("already on target branch '%s'", request.TargetBranch)
		return result, nil
	}

	// Get commits count before rebase
	commitsBefore, err := ws.deps.Git.GetCommitsAhead(ctx, repoPath, request.TargetBranch)
	if err != nil {
		ws.deps.Logger.Warn("Could not get commits count before rebase",
			ux.Field("repository", repoName),
			ux.Field("error", err))
	}
	result.CommitsBefore = commitsBefore

	if request.DryRun {
		result.Error = "dry-run mode"
		return result, nil
	}

	// Check if target branch exists
	exists, err := ws.deps.Git.BranchExists(ctx, repoPath, request.TargetBranch)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to check if target branch exists: %v", err)
		return result, nil
	}

	if !exists {
		// Try to fetch it from remote
		if err := ws.deps.Git.FetchBranch(ctx, repoPath, request.TargetBranch); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("target branch '%s' not found locally or on remote", request.TargetBranch)
			return result, nil
		}
	}

	// Perform rebase
	if err := ws.deps.Git.Rebase(ctx, repoPath, request.TargetBranch, request.Interactive); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("rebase failed: %v", err)

		// Check for conflicts
		hasConflicts, conflictErr := ws.deps.Git.HasRebaseConflicts(ctx, repoPath)
		if conflictErr != nil {
			ws.deps.Logger.Warn("Could not check for rebase conflicts",
				ux.Field("repository", repoName),
				ux.Field("error", conflictErr))
		} else {
			result.Conflicts = hasConflicts
		}
		return result, nil
	}

	result.Rebased = true

	// Get commits count after rebase
	commitsAfter, err := ws.deps.Git.GetCommitsAhead(ctx, repoPath, request.TargetBranch)
	if err != nil {
		ws.deps.Logger.Warn("Could not get commits count after rebase",
			ux.Field("repository", repoName),
			ux.Field("error", err))
	}
	result.CommitsAfter = commitsAfter

	ws.deps.Logger.Info("Repository rebase completed",
		ux.Field("repository", repoName),
		ux.Field("target_branch", request.TargetBranch),
		ux.Field("commits_before", result.CommitsBefore),
		ux.Field("commits_after", result.CommitsAfter))

	return result, nil
}

// DetectWorkspaceFromPath detects workspace structure without wsm.json for backwards compatibility
func (s *WorkspaceService) DetectWorkspaceFromPath(absPath string) (*domain.Workspace, error) {
	// Walk up the directory tree to find workspace structure
	for dir := absPath; dir != "/" && dir != ""; dir = filepath.Dir(dir) {
		// Check if this directory contains repository worktrees
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		// Look for .git files (worktree indicators)
		gitDirs := 0
		var gitRepoPaths []string
		for _, entry := range entries {
			if entry.IsDir() {
				gitFile := filepath.Join(dir, entry.Name(), ".git")
				if stat, err := os.Stat(gitFile); err == nil && stat.Mode().IsRegular() {
					gitDirs++
					gitRepoPaths = append(gitRepoPaths, filepath.Join(dir, entry.Name()))
				}
			}
		}

		// If we found multiple git worktrees, this might be a workspace
		if gitDirs >= 2 {
			// Create a minimal workspace structure for backwards compatibility
			workspace := &domain.Workspace{
				Name:         filepath.Base(dir),
				Path:         dir,
				Repositories: make([]domain.Repository, 0, len(gitRepoPaths)),
			}

			// Analyze each repository to populate proper git information
			ctx := context.Background()
			for _, repoPath := range gitRepoPaths {
				repo, err := s.analyzeRepositoryForBackwardsCompatibility(ctx, repoPath)
				if err != nil {
					s.deps.Logger.Debug("Failed to analyze repository, using minimal info",
						ux.Field("path", repoPath),
						ux.Field("error", err))
					// Fallback to minimal repository info
					repo = &domain.Repository{
						Name: filepath.Base(repoPath),
						Path: repoPath,
					}
				}
				workspace.Repositories = append(workspace.Repositories, *repo)
			}

			// Create metadata file for backwards compatibility
			if err := s.createWorkspaceMetadata(workspace); err != nil {
				return nil, errors.Wrap(err, "failed to create workspace metadata")
			}

			return workspace, nil
		}

		// Stop at parent directory to avoid infinite loop
		if filepath.Dir(dir) == dir {
			break
		}
	}

	return nil, errors.New("no workspace structure detected")
}

// analyzeRepositoryForBackwardsCompatibility analyzes a git repository to populate proper information
func (s *WorkspaceService) analyzeRepositoryForBackwardsCompatibility(ctx context.Context, repoPath string) (*domain.Repository, error) {
	name := filepath.Base(repoPath)

	// Get remote URL
	remoteURL, err := s.deps.Git.RemoteURL(ctx, repoPath)
	if err != nil {
		s.deps.Logger.Debug("Failed to get remote URL",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		remoteURL = ""
	}

	// Get current branch
	currentBranch, err := s.deps.Git.CurrentBranch(ctx, repoPath)
	if err != nil {
		s.deps.Logger.Debug("Failed to get current branch",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		currentBranch = ""
	}

	// Get branches
	branches, err := s.deps.Git.Branches(ctx, repoPath)
	if err != nil {
		s.deps.Logger.Debug("Failed to get branches",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		branches = []string{}
	}

	// Get tags
	tags, err := s.deps.Git.Tags(ctx, repoPath)
	if err != nil {
		s.deps.Logger.Debug("Failed to get tags",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		tags = []string{}
	}

	// Get last commit
	lastCommit, err := s.deps.Git.LastCommit(ctx, repoPath)
	if err != nil {
		s.deps.Logger.Debug("Failed to get last commit",
			ux.Field("path", repoPath),
			ux.Field("error", err))
		lastCommit = ""
	}

	return &domain.Repository{
		Name:          name,
		Path:          repoPath,
		RemoteURL:     remoteURL,
		CurrentBranch: currentBranch,
		Branches:      branches,
		Tags:          tags,
		LastCommit:    lastCommit,
		LastUpdated:   time.Now(),
		Categories:    []string{}, // Empty for backwards compatibility
	}, nil
}

// createWorkspaceMetadata creates the .wsm/wsm.json file for backwards compatibility
func (s *WorkspaceService) createWorkspaceMetadata(workspace *domain.Workspace) error {
	metadataDir := filepath.Dir(workspace.MetadataPath())

	// Create .wsm directory if it doesn't exist
	if !s.deps.FS.Exists(metadataDir) {
		if err := s.deps.FS.MkdirAll(metadataDir, 0755); err != nil {
			return errors.Wrap(err, "failed to create .wsm directory")
		}
	}

	// Create metadata content
	metadataBytes, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal workspace metadata")
	}

	// Write metadata file
	if err := s.deps.FS.WriteFile(workspace.MetadataPath(), metadataBytes, 0644); err != nil {
		return errors.Wrap(err, "failed to write workspace metadata")
	}

	return nil
}

// LoadWorkspaceFromPath loads a workspace from the given path, with backwards compatibility
func (s *WorkspaceService) LoadWorkspaceFromPath(workspacePath string) (*domain.Workspace, error) {
	// Make path absolute
	absPath, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get absolute path")
	}

	// Look for .wsm/wsm.json metadata file
	metadataPath := s.deps.FS.Join(absPath, ".wsm", "wsm.json")

	if !s.deps.FS.Exists(metadataPath) {
		// Try to find workspace metadata by walking up the directory tree
		for dir := absPath; dir != "/" && dir != ""; dir = filepath.Dir(dir) {
			metadataPath = s.deps.FS.Join(dir, ".wsm", "wsm.json")
			if s.deps.FS.Exists(metadataPath) {
				absPath = dir
				break
			}
			// Stop at parent directory to avoid infinite loop
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}

	if s.deps.FS.Exists(metadataPath) {
		// Load and parse metadata
		metadataBytes, err := s.deps.FS.ReadFile(metadataPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read workspace metadata")
		}

		var workspace domain.Workspace
		if err := json.Unmarshal(metadataBytes, &workspace); err != nil {
			return nil, errors.Wrap(err, "failed to parse workspace metadata")
		}

		// Ensure path is set correctly
		workspace.Path = absPath

		return &workspace, nil
	}

	// Backwards compatibility: Try to detect workspace without wsm.json
	detectedWorkspace, err := s.DetectWorkspaceFromPath(absPath)
	if err != nil {
		return nil, errors.Errorf("no workspace found at '%s' or any parent directory. Expected .wsm/wsm.json file or workspace with multiple git repositories", workspacePath)
	}

	return detectedWorkspace, nil
}
