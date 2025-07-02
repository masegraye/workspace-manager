package service

import (
	"context"
	"path/filepath"

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
