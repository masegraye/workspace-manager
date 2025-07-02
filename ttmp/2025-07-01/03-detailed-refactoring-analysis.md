# WSM Refactoring Analysis: Current State vs. Proposed Architecture

## Executive Summary

After analyzing the actual source code against the O3 Pro refactoring plan, here's the current state and a detailed roadmap for implementing the proposed clean architecture.

## Current Architecture Analysis

### Current Structure

```
pkg/wsm/
├─ types.go           (73 lines) - Clean domain model ✓
├─ workspace.go       (1863 lines) - MASSIVE GOD FILE ❌
├─ discovery.go       (394 lines) - Repository discovery logic
├─ git_operations.go  (378 lines) - Git operations for commits/staging
├─ git_utils.go       (93 lines) - Git utility functions
├─ status.go          (210 lines) - Status checking logic
├─ sync_operations.go (395 lines) - Sync and branch operations
├─ utils.go           (20 lines) - Small utilities
└─ output/styles.go   - Output styling
```

### Key Issues Identified

1. **God File**: `workspace.go` has 1,863 lines containing:
   - WorkspaceManager struct and construction
   - Workspace CRUD operations
   - Worktree management
   - File system operations
   - Git command execution
   - User prompting (via fmt.Printf)
   - Setup script execution
   - Configuration management

2. **Direct Dependencies**: Found 47 instances of `exec.Command` and 34 instances of `os.*` calls scattered throughout
3. **Mixed Concerns**: Business logic mixed with I/O, prompting, and side effects
4. **No Dependency Injection**: Everything is tightly coupled
5. **No Testability**: Cannot unit test business logic without executing git commands

### Current CLI Structure
- Clean command separation in `cmd/cmds/` ✓
- Root command in `cmd/wsm/root.go`
- Each command is properly separated (20+ command files)

## Proposed Architecture Implementation Plan

### Phase 1: Extract Interfaces & Create Adapters (Week 1)

#### 1.1 Create Core Interfaces

```go
// pkg/wsm/fs/file_system.go
package fs

type FileSystem interface {
    MkdirAll(path string, perm os.FileMode) error
    RemoveAll(path string) error
    WriteFile(filename string, data []byte, perm os.FileMode) error
    ReadFile(filename string) ([]byte, error)
    ReadDir(dirname string) ([]os.DirEntry, error)
    Stat(name string) (fs.FileInfo, error)
    UserConfigDir() (string, error)
    UserHomeDir() (string, error)
}

type OSFileSystem struct{}
func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
// ... implement all methods
```

#### 1.2 Create Git Client Interface

```go
// pkg/wsm/git/client.go
package git

type Client interface {
    // Worktree operations
    WorktreeAdd(ctx context.Context, repoPath, branch, targetPath string, opts WorktreeAddOpts) error
    WorktreeRemove(ctx context.Context, repoPath, targetPath string, force bool) error
    WorktreeList(ctx context.Context, repoPath string) ([]WorktreeInfo, error)
    
    // Branch operations
    BranchExists(ctx context.Context, repoPath, branch string) (bool, error)
    RemoteBranchExists(ctx context.Context, repoPath, branch string) (bool, error)
    CurrentBranch(ctx context.Context, repoPath string) (string, error)
    
    // Status and changes
    Status(ctx context.Context, repoPath string) (*StatusInfo, error)
    AheadBehind(ctx context.Context, repoPath string) (ahead, behind int, err error)
    HasChanges(ctx context.Context, repoPath string) (bool, error)
    UntrackedFiles(ctx context.Context, repoPath string) ([]string, error)
    
    // Operations
    Add(ctx context.Context, repoPath, filePath string) error
    Commit(ctx context.Context, repoPath, message string) error
    Push(ctx context.Context, repoPath string) error
    Pull(ctx context.Context, repoPath string, rebase bool) error
    Fetch(ctx context.Context, repoPath string) error
    
    // Repository info
    RemoteURL(ctx context.Context, repoPath string) (string, error)
    Branches(ctx context.Context, repoPath string) ([]string, error)
    Tags(ctx context.Context, repoPath string) ([]string, error)
    LastCommit(ctx context.Context, repoPath string) (string, error)
}

type WorktreeAddOpts struct {
    Force     bool
    Track     string
    NewBranch bool
}

type WorktreeInfo struct {
    Path   string
    Branch string
    Commit string
}

type StatusInfo struct {
    StagedFiles    []string
    ModifiedFiles  []string
    UntrackedFiles []string
    HasConflicts   bool
}
```

#### 1.3 Create UX Interfaces

```go
// pkg/wsm/ux/interfaces.go
package ux

type Prompter interface {
    Select(message string, options []string) (string, error)
    Confirm(message string) (bool, error)
    Input(message string) (string, error)
}

type Logger interface {
    Info(msg string, fields ...LogField)
    Warn(msg string, fields ...LogField)
    Error(msg string, fields ...LogField)
    Debug(msg string, fields ...LogField)
}

type LogField struct {
    Key   string
    Value interface{}
}

func Field(key string, value interface{}) LogField {
    return LogField{Key: key, Value: value}
}
```

### Phase 2: Move Domain Model (Week 1)

```bash
mkdir -p pkg/wsm/domain
mv pkg/wsm/types.go pkg/wsm/domain/types.go
```

Add pure helper methods:
```go
// pkg/wsm/domain/workspace.go
package domain

func (w Workspace) NeedsGoWorkspace() bool {
    for _, repo := range w.Repositories {
        for _, category := range repo.Categories {
            if category == "go" {
                return true
            }
        }
    }
    return false
}

func (w Workspace) MetadataPath() string {
    return filepath.Join(w.Path, ".wsm", "wsm.json")
}

func (w Workspace) GoWorkPath() string {
    return filepath.Join(w.Path, "go.work")
}
```

### Phase 3: Extract Service Dependencies (Week 2)

#### 3.1 Create Dependencies Container

```go
// pkg/wsm/service/deps.go
package service

type Deps struct {
    FS       fs.FileSystem
    Git      git.Client
    Prompter ux.Prompter
    Logger   ux.Logger
    Clock    func() time.Time
}

func NewDeps() *Deps {
    return &Deps{
        FS:       fs.OSFileSystem{},
        Git:      git.NewExecClient(),
        Prompter: ux.NewHuhPrompter(),
        Logger:   ux.NewStructuredLogger(),
        Clock:    time.Now,
    }
}
```

#### 3.2 Extract Worktree Service

```go
// pkg/wsm/worktree/service.go
package worktree

type Service struct {
    git git.Client
    log ux.Logger
}

func New(git git.Client, log ux.Logger) *Service {
    return &Service{git: git, log: log}
}

func (s *Service) Create(ctx context.Context, repo domain.Repository, targetPath, branch string, opts CreateOpts) error {
    s.log.Info("Creating worktree", 
        ux.Field("repo", repo.Name),
        ux.Field("branch", branch),
        ux.Field("target", targetPath))
    
    // Determine strategy based on branch existence
    if opts.Force {
        return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{
            Force:     true,
            NewBranch: true,
        })
    }
    
    // Check if branch exists locally or remotely
    localExists, err := s.git.BranchExists(ctx, repo.Path, branch)
    if err != nil {
        return err
    }
    
    remoteExists, err := s.git.RemoteBranchExists(ctx, repo.Path, branch)
    if err != nil {
        return err
    }
    
    if localExists {
        return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{})
    } else if remoteExists {
        return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{
            Track: "origin/" + branch,
        })
    } else {
        return s.git.WorktreeAdd(ctx, repo.Path, branch, targetPath, git.WorktreeAddOpts{
            NewBranch: true,
        })
    }
}

func (s *Service) Remove(ctx context.Context, repo domain.Repository, targetPath string, force bool) error {
    return s.git.WorktreeRemove(ctx, repo.Path, targetPath, force)
}
```

#### 3.3 Extract Metadata Service

```go
// pkg/wsm/metadata/builder.go
package metadata

type Builder struct {
    clock func() time.Time
}

func New(clock func() time.Time) *Builder {
    return &Builder{clock: clock}
}

func (b *Builder) BuildWorkspaceMetadata(ws domain.Workspace) ([]byte, error) {
    metadata := WorkspaceMetadata{
        Name:        ws.Name,
        Path:        ws.Path,
        Branch:      ws.Branch,
        BaseBranch:  ws.BaseBranch,
        GoWorkspace: ws.GoWorkspace,
        AgentMD:     ws.AgentMD,
        CreatedAt:   ws.Created,
        Environment: make(map[string]string),
    }
    
    for _, repo := range ws.Repositories {
        metadata.Repositories = append(metadata.Repositories, RepositoryMetadata{
            Name:         repo.Name,
            Path:         repo.Path,
            Categories:   repo.Categories,
            WorktreePath: filepath.Join(ws.Path, repo.Name),
        })
    }
    
    return json.MarshalIndent(metadata, "", "  ")
}
```

#### 3.4 Extract Go.work Generator

```go
// pkg/wsm/gowork/generator.go
package gowork

func Generate(workspace domain.Workspace) string {
    var content strings.Builder
    content.WriteString("go 1.21\n\n")
    content.WriteString("use (\n")
    
    for _, repo := range workspace.Repositories {
        for _, category := range repo.Categories {
            if category == "go" {
                content.WriteString(fmt.Sprintf("    ./%s\n", repo.Name))
                break
            }
        }
    }
    
    content.WriteString(")\n")
    return content.String()
}
```

### Phase 4: Create High-Level Services (Week 2-3)

#### 4.1 Workspace Service

```go
// pkg/wsm/service/workspace.go
package service

type WorkspaceService struct {
    deps      *Deps
    worktree  *worktree.Service
    metadata  *metadata.Builder
    gowork    *gowork.Generator
    discovery *discovery.Service
    config    *config.Service
}

func NewWorkspaceService(deps *Deps) *WorkspaceService {
    return &WorkspaceService{
        deps:      deps,
        worktree:  worktree.New(deps.Git, deps.Logger),
        metadata:  metadata.New(deps.Clock),
        gowork:    gowork.New(),
        discovery: discovery.New(deps.FS, deps.Git, deps.Logger),
        config:    config.New(deps.FS),
    }
}

type CreateRequest struct {
    Name       string
    RepoNames  []string
    Branch     string
    BaseBranch string
    AgentMD    string
    DryRun     bool
}

func (s *WorkspaceService) Create(ctx context.Context, req CreateRequest) (*domain.Workspace, error) {
    s.deps.Logger.Info("Creating workspace", ux.Field("name", req.Name))
    
    // 1. Load configuration and validate
    cfg, err := s.config.Load()
    if err != nil {
        return nil, errors.Wrap(err, "failed to load config")
    }
    
    workspacePath := filepath.Join(cfg.WorkspaceDir, req.Name)
    
    // 2. Find repositories
    repos, err := s.discovery.FindRepositories(req.RepoNames)
    if err != nil {
        return nil, errors.Wrap(err, "failed to find repositories")
    }
    
    // 3. Build workspace object
    workspace := &domain.Workspace{
        Name:         req.Name,
        Path:         workspacePath,
        Repositories: repos,
        Branch:       req.Branch,
        BaseBranch:   req.BaseBranch,
        Created:      s.deps.Clock(),
        GoWorkspace:  domain.NeedsGoWorkspace(repos),
        AgentMD:      req.AgentMD,
    }
    
    if req.DryRun {
        return workspace, nil
    }
    
    // 4. Create physical structure
    if err := s.createPhysicalStructure(ctx, workspace); err != nil {
        return nil, err
    }
    
    // 5. Save configuration
    if err := s.config.SaveWorkspace(workspace); err != nil {
        return nil, errors.Wrap(err, "failed to save workspace")
    }
    
    return workspace, nil
}

func (s *WorkspaceService) createPhysicalStructure(ctx context.Context, ws *domain.Workspace) error {
    // Create workspace directory
    if err := s.deps.FS.MkdirAll(ws.Path, 0755); err != nil {
        return errors.Wrap(err, "failed to create workspace directory")
    }
    
    // Track created worktrees for rollback
    var created []domain.Repository
    
    // Create worktrees
    for _, repo := range ws.Repositories {
        targetPath := filepath.Join(ws.Path, repo.Name)
        
        if err := s.worktree.Create(ctx, repo, targetPath, ws.Branch, worktree.CreateOpts{}); err != nil {
            s.rollback(ctx, ws.Path, created)
            return errors.Wrapf(err, "failed to create worktree for %s", repo.Name)
        }
        
        created = append(created, repo)
    }
    
    // Create go.work if needed
    if ws.GoWorkspace {
        content := s.gowork.Generate(*ws)
        if err := s.deps.FS.WriteFile(ws.GoWorkPath(), []byte(content), 0644); err != nil {
            s.rollback(ctx, ws.Path, created)
            return errors.Wrap(err, "failed to create go.work")
        }
    }
    
    // Create metadata
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
    
    return nil
}

func (s *WorkspaceService) rollback(ctx context.Context, workspacePath string, created []domain.Repository) {
    s.deps.Logger.Warn("Rolling back workspace creation", 
        ux.Field("workspace", workspacePath),
        ux.Field("worktrees", len(created)))
    
    for _, repo := range created {
        targetPath := filepath.Join(workspacePath, repo.Name)
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
```

### Phase 5: Update CLI Layer (Week 3)

```go
// cmd/cmds/cmd_create.go (updated)
func newCreateCommand() *cobra.Command {
    var (
        branch     string
        baseBranch string
        agentMD    string
        dryRun     bool
    )
    
    cmd := &cobra.Command{
        Use:   "create <workspace-name> <repo1> [repo2] [repo3]...",
        Short: "Create a new workspace",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Build dependencies
            deps := service.NewDeps()
            workspaceService := service.NewWorkspaceService(deps)
            
            // Handle interactive prompting for conflicts
            if !dryRun {
                // Pre-validate and prompt for conflicts
                if err := handleCreateConflicts(deps.Prompter, args); err != nil {
                    return err
                }
            }
            
            // Execute service
            workspace, err := workspaceService.Create(cmd.Context(), service.CreateRequest{
                Name:       args[0],
                RepoNames:  args[1:],
                Branch:     branch,
                BaseBranch: baseBranch,
                AgentMD:    agentMD,
                DryRun:     dryRun,
            })
            
            if err != nil {
                return err
            }
            
            if dryRun {
                return printWorkspacePreview(workspace)
            }
            
            deps.Logger.Info("Workspace created successfully", 
                ux.Field("name", workspace.Name),
                ux.Field("path", workspace.Path))
            
            return nil
        },
    }
    
    // Add flags...
    return cmd
}

func handleCreateConflicts(prompter ux.Prompter, args []string) error {
    // Check for branch conflicts and prompt user
    // This is where all user interaction happens
    return nil
}
```

### Phase 6: Migration Steps (Week 4)

#### 6.1 Migration Checklist

- [ ] **Day 1-2**: Create interface packages (`fs`, `git`, `ux`)
- [ ] **Day 3-4**: Move domain model to `domain/` package
- [ ] **Day 5-7**: Extract `worktree.Service` from workspace.go
- [ ] **Day 8-10**: Extract `metadata.Builder` and `gowork.Generator`
- [ ] **Day 11-14**: Create `WorkspaceService` orchestrator
- [ ] **Day 15-17**: Update CLI commands to use services
- [ ] **Day 18-20**: Remove old code and cleanup

#### 6.2 Immediate Implementation Commands

```bash
# Create new package structure
mkdir -p pkg/wsm/{domain,fs,git,ux,worktree,metadata,gowork,config,service}

# Move domain types
mv pkg/wsm/types.go pkg/wsm/domain/types.go

# Update all imports
find . -name "*.go" -exec sed -i 's|github.com/go-go-golems/workspace-manager/pkg/wsm|github.com/go-go-golems/workspace-manager/pkg/wsm/domain|g' {} \;
```

### Testing Strategy

#### Unit Tests
```go
// pkg/wsm/worktree/service_test.go
func TestService_Create(t *testing.T) {
    mockGit := &git.MockClient{}
    mockLog := &ux.MockLogger{}
    service := worktree.New(mockGit, mockLog)
    
    // Test scenarios with mocked dependencies
}
```

#### Integration Tests
```go
// integration_test.go
func TestWorkspaceService_Create_Integration(t *testing.T) {
    tempDir := t.TempDir()
    deps := &service.Deps{
        FS:  fs.NewMemoryFS(), // Use memory filesystem
        Git: git.NewFakeClient(tempDir), // Use fake git with temp repos
        // ...
    }
    
    service := service.NewWorkspaceService(deps)
    // Test full workflow
}
```

## Benefits After Refactoring

1. **Maintainability**: `workspace.go` reduced from 1,863 lines to ~150 lines
2. **Testability**: 90%+ unit test coverage without external dependencies
3. **Extensibility**: New features can be added as services without touching core logic
4. **Debugging**: Clear separation makes issues easier to isolate
5. **Team Development**: Multiple developers can work on different services independently

## Risks & Mitigation

1. **Breaking Changes**: Use feature flags during migration
2. **Import Path Changes**: Update gradually with aliases
3. **Performance**: Benchmark critical paths during refactoring
4. **Complexity**: Keep interfaces small and focused

## Conclusion

The current codebase has good separation at the CLI level but suffers from a massive god file in `workspace.go`. The proposed refactoring will create a clean, testable, and maintainable architecture while preserving all existing functionality.

The phased approach ensures we can deliver value incrementally and reduce risks through gradual migration. 