package service

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/git"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
)

// MockFileSystem implements our fs.FileSystem interface for testing
type MockFileSystem struct {
	files map[string][]byte
	dirs  map[string]bool
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *MockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	m.dirs[path] = true
	return nil
}

func (m *MockFileSystem) RemoveAll(path string) error {
	delete(m.dirs, path)
	for file := range m.files {
		if strings.HasPrefix(file, path) {
			delete(m.files, file)
		}
	}
	return nil
}

func (m *MockFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	m.files[filename] = data
	return nil
}

func (m *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if data, exists := m.files[filename]; exists {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) ReadDir(dirname string) ([]os.DirEntry, error) {
	return nil, nil // Simplified for this example
}

func (m *MockFileSystem) Stat(name string) (fs.FileInfo, error) {
	return nil, nil // Simplified for this example
}

func (m *MockFileSystem) UserConfigDir() (string, error) {
	return "/home/user/.config", nil
}

func (m *MockFileSystem) UserHomeDir() (string, error) {
	return "/home/user", nil
}

func (m *MockFileSystem) Exists(path string) bool {
	_, exists := m.files[path]
	return exists || m.dirs[path]
}

func (m *MockFileSystem) Join(elem ...string) string {
	return strings.Join(elem, "/")
}

// MockGitClient implements git.Client for testing
type MockGitClient struct {
	worktrees map[string][]git.WorktreeInfo
}

func NewMockGitClient() *MockGitClient {
	return &MockGitClient{
		worktrees: make(map[string][]git.WorktreeInfo),
	}
}

func (m *MockGitClient) WorktreeAdd(ctx context.Context, repoPath, branch, targetPath string, opts git.WorktreeAddOpts) error {
	// Simulate successful worktree creation
	return nil
}

func (m *MockGitClient) WorktreeRemove(ctx context.Context, repoPath, targetPath string, force bool) error {
	return nil
}

func (m *MockGitClient) WorktreeList(ctx context.Context, repoPath string) ([]git.WorktreeInfo, error) {
	return m.worktrees[repoPath], nil
}

// Implement other required methods with simple stubs
func (m *MockGitClient) BranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	return false, nil
}

func (m *MockGitClient) RemoteBranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	return false, nil
}

func (m *MockGitClient) CurrentBranch(ctx context.Context, repoPath string) (string, error) {
	return "main", nil
}

func (m *MockGitClient) Status(ctx context.Context, repoPath string) (*git.StatusInfo, error) {
	return &git.StatusInfo{Clean: true}, nil
}

func (m *MockGitClient) AheadBehind(ctx context.Context, repoPath string) (ahead, behind int, err error) {
	return 0, 0, nil
}

func (m *MockGitClient) HasChanges(ctx context.Context, repoPath string) (bool, error) {
	return false, nil
}

func (m *MockGitClient) UntrackedFiles(ctx context.Context, repoPath string) ([]string, error) {
	return []string{}, nil
}

func (m *MockGitClient) Add(ctx context.Context, repoPath, filePath string) error {
	return nil
}

func (m *MockGitClient) Commit(ctx context.Context, repoPath, message string) error {
	return nil
}

func (m *MockGitClient) Push(ctx context.Context, repoPath string) error {
	return nil
}

func (m *MockGitClient) Pull(ctx context.Context, repoPath string, rebase bool) error {
	return nil
}

func (m *MockGitClient) Fetch(ctx context.Context, repoPath string) error {
	return nil
}

func (m *MockGitClient) RemoteURL(ctx context.Context, repoPath string) (string, error) {
	return "https://github.com/example/repo.git", nil
}

func (m *MockGitClient) Branches(ctx context.Context, repoPath string) ([]string, error) {
	return []string{"main", "develop"}, nil
}

func (m *MockGitClient) Tags(ctx context.Context, repoPath string) ([]string, error) {
	return []string{"v1.0.0"}, nil
}

func (m *MockGitClient) LastCommit(ctx context.Context, repoPath string) (string, error) {
	return "abc123", nil
}

func (m *MockGitClient) IsRepository(ctx context.Context, path string) (bool, error) {
	return true, nil
}

// MockLogger implements ux.Logger for testing
type MockLogger struct {
	messages []string
}

func NewMockLogger() *MockLogger {
	return &MockLogger{}
}

func (m *MockLogger) Info(msg string, fields ...ux.LogField) {
	m.messages = append(m.messages, "INFO: "+msg)
}

func (m *MockLogger) Warn(msg string, fields ...ux.LogField) {
	m.messages = append(m.messages, "WARN: "+msg)
}

func (m *MockLogger) Error(msg string, fields ...ux.LogField) {
	m.messages = append(m.messages, "ERROR: "+msg)
}

func (m *MockLogger) Debug(msg string, fields ...ux.LogField) {
	m.messages = append(m.messages, "DEBUG: "+msg)
}

// TestWorkspaceService_Create demonstrates how easy it is to test the new architecture
func TestWorkspaceService_Create(t *testing.T) {
	// Arrange
	mockFS := NewMockFileSystem()
	mockGit := NewMockGitClient()
	mockLogger := NewMockLogger()
	
	// Set up default config file
	configPath := "/home/user/.config/wsm/config.json"
	configData := `{
		"workspace_dir": "/home/user/workspaces",
		"template_dir": "/home/user/.config/wsm/templates",
		"registry_path": "/home/user/.config/wsm/registry.json"
	}`
	mockFS.WriteFile(configPath, []byte(configData), 0644)
	
	deps := &Deps{
		FS:       mockFS,
		Git:      mockGit,
		Prompter: nil, // Not needed for this test
		Logger:   mockLogger,
		Clock:    func() time.Time { return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
	
	service := NewWorkspaceService(deps)
	
	// Act
	workspace, err := service.Create(context.Background(), CreateRequest{
		Name:      "test-workspace",
		RepoNames: []string{"repo1", "repo2"},
		Branch:    "feature/test",
		DryRun:    false,
	})
	
	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if workspace.Name != "test-workspace" {
		t.Errorf("Expected workspace name 'test-workspace', got %s", workspace.Name)
	}
	
	expectedPath := "/home/user/workspaces/test-workspace"
	if workspace.Path != expectedPath {
		t.Errorf("Expected workspace path %s, got %s", expectedPath, workspace.Path)
	}
	
	if len(workspace.Repositories) != 2 {
		t.Errorf("Expected 2 repositories, got %d", len(workspace.Repositories))
	}
	
	// Verify metadata was created
	metadataPath := "/home/user/workspaces/test-workspace/.wsm/wsm.json"
	if !mockFS.Exists(metadataPath) {
		t.Error("Expected metadata file to be created")
	}
	
	// Verify directories were created
	if !mockFS.Exists("/home/user/workspaces/test-workspace") {
		t.Error("Expected workspace directory to be created")
	}
	
	// Check that logger was used
	if len(mockLogger.messages) == 0 {
		t.Error("Expected log messages to be written")
	}
}

func TestWorkspaceService_Create_DryRun(t *testing.T) {
	// Arrange
	mockFS := NewMockFileSystem()
	mockGit := NewMockGitClient()
	mockLogger := NewMockLogger()
	
	deps := &Deps{
		FS:       mockFS,
		Git:      mockGit,
		Prompter: nil,
		Logger:   mockLogger,
		Clock:    func() time.Time { return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
	
	service := NewWorkspaceService(deps)
	
	// Act
	workspace, err := service.Create(context.Background(), CreateRequest{
		Name:      "test-workspace",
		RepoNames: []string{"repo1"},
		Branch:    "feature/test",
		DryRun:    true,
	})
	
	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if workspace == nil {
		t.Fatal("Expected workspace to be returned")
	}
	
	// Verify nothing was actually created in dry run mode
	if len(mockFS.files) > 0 {
		t.Error("Expected no files to be created in dry run mode")
	}
	
	if len(mockFS.dirs) > 0 {
		t.Error("Expected no directories to be created in dry run mode")
	}
}
