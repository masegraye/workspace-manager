package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/sync"
)

// TestWorkspaceService_Integration demonstrates a full workflow using the new architecture
func TestWorkspaceService_Integration(t *testing.T) {
	// Arrange
	mockFS := NewMockFileSystem()
	mockGit := NewMockGitClient()
	mockLogger := NewMockLogger()

	// Set up config
	configPath := "/home/user/.config/wsm/config.json"
	configData := `{
		"workspace_dir": "/home/user/workspaces",
		"template_dir": "/home/user/.config/wsm/templates",
		"registry_path": "/home/user/.config/wsm/registry.json"
	}`
	if err := mockFS.WriteFile(configPath, []byte(configData), 0644); err != nil {
		panic(err)
	}

	// Set up registry with some repositories
	registryPath := "/home/user/.config/wsm/registry.json"
	registryData := `{
		"repositories": [
			{
				"name": "repo1",
				"path": "/source/repo1",
				"categories": ["go"],
				"remote_url": "https://github.com/example/repo1.git",
				"current_branch": "main"
			},
			{
				"name": "repo2", 
				"path": "/source/repo2",
				"categories": ["nodejs"],
				"remote_url": "https://github.com/example/repo2.git",
				"current_branch": "main"
			}
		],
		"last_scan": "2023-01-01T00:00:00Z"
	}`
	if err := mockFS.WriteFile(registryPath, []byte(registryData), 0644); err != nil {
		panic(err)
	}

	deps := &Deps{
		FS:       mockFS,
		Git:      mockGit,
		Prompter: nil,
		Logger:   mockLogger,
		Clock:    func() time.Time { return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	service := NewWorkspaceService(deps)

	// Act 1: Create a workspace
	workspace, err := service.Create(context.Background(), CreateRequest{
		Name:      "integration-test",
		RepoNames: []string{"repo1", "repo2"},
		Branch:    "feature/integration",
		DryRun:    false,
	})

	// Assert workspace creation
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	if workspace.Name != "integration-test" {
		t.Errorf("Expected workspace name 'integration-test', got %s", workspace.Name)
	}

	if len(workspace.Repositories) != 2 {
		t.Errorf("Expected 2 repositories, got %d", len(workspace.Repositories))
	}

	// Act 2: Get workspace status
	status, err := service.GetWorkspaceStatus(context.Background(), *workspace)
	if err != nil {
		t.Fatalf("Failed to get workspace status: %v", err)
	}

	// Assert status
	if status.Workspace.Name != workspace.Name {
		t.Errorf("Expected status workspace name %s, got %s", workspace.Name, status.Workspace.Name)
	}

	if len(status.Repositories) != 2 {
		t.Errorf("Expected 2 repository statuses, got %d", len(status.Repositories))
	}

	// Act 3: Sync workspace (dry run)
	syncResults, err := service.SyncWorkspace(context.Background(), *workspace, sync.SyncOptions{
		Pull:   true,
		Push:   false,
		Rebase: false,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Failed to sync workspace: %v", err)
	}

	// Assert sync results
	if len(syncResults) != 2 {
		t.Errorf("Expected 2 sync results, got %d", len(syncResults))
	}

	for _, result := range syncResults {
		if result.Error != "dry-run mode" {
			t.Errorf("Expected dry-run mode message, got %s", result.Error)
		}
	}

	// Act 4: List repositories
	repos, err := service.ListRepositories()
	if err != nil {
		t.Fatalf("Failed to list repositories: %v", err)
	}

	// Assert repository list
	if len(repos) < 2 {
		t.Errorf("Expected at least 2 repositories, got %d", len(repos))
	}

	// Verify that the workspace structure was created
	expectedFiles := []string{
		"/home/user/workspaces/integration-test/.wsm/wsm.json",
		"/home/user/workspaces/integration-test/go.work", // Should be created because repo1 is Go
	}

	for _, file := range expectedFiles {
		if !mockFS.Exists(file) {
			t.Errorf("Expected file %s to exist", file)
		}
	}

	// Verify workspace metadata content
	metadataBytes, err := mockFS.ReadFile("/home/user/workspaces/integration-test/.wsm/wsm.json")
	if err != nil {
		t.Fatalf("Failed to read workspace metadata: %v", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatalf("Failed to parse workspace metadata: %v", err)
	}

	if metadata["name"] != "integration-test" {
		t.Errorf("Expected metadata name 'integration-test', got %v", metadata["name"])
	}

	if metadata["go_workspace"] != true {
		t.Errorf("Expected go_workspace to be true, got %v", metadata["go_workspace"])
	}

	// Verify go.work content
	goWorkBytes, err := mockFS.ReadFile("/home/user/workspaces/integration-test/go.work")
	if err != nil {
		t.Fatalf("Failed to read go.work file: %v", err)
	}

	goWorkContent := string(goWorkBytes)
	if !strings.Contains(goWorkContent, "./repo1") {
		t.Errorf("Expected go.work to contain './repo1', got: %s", goWorkContent)
	}

	// Should not contain repo2 since it's not a Go project
	if strings.Contains(goWorkContent, "./repo2") {
		t.Errorf("Expected go.work to NOT contain './repo2', got: %s", goWorkContent)
	}

	// Verify logging occurred
	if len(mockLogger.messages) == 0 {
		t.Error("Expected log messages to be written")
	}

	// Check for specific log messages
	hasCreationLog := false
	for _, msg := range mockLogger.messages {
		if strings.Contains(msg, "Creating workspace") {
			hasCreationLog = true
			break
		}
	}
	if !hasCreationLog {
		t.Error("Expected to find workspace creation log message")
	}
}

// TestWorkspaceService_Discovery tests the discovery functionality
func TestWorkspaceService_Discovery(t *testing.T) {
	// Arrange
	mockFS := NewMockFileSystem()
	mockGit := NewMockGitClient()
	mockLogger := NewMockLogger()

	// Set up mock filesystem with some project directories
	mockFS.dirs["/projects/go-app"] = true
	mockFS.files["/projects/go-app/go.mod"] = []byte("module example.com/go-app")

	mockFS.dirs["/projects/node-app"] = true
	mockFS.files["/projects/node-app/package.json"] = []byte(`{"name": "node-app"}`)

	mockFS.dirs["/projects/python-app"] = true
	mockFS.files["/projects/python-app/requirements.txt"] = []byte("flask==2.0.0")

	// Configure mock git to return true for IsRepository
	mockGit.isRepoResponse = map[string]bool{
		"/projects/go-app":     true,
		"/projects/node-app":   true,
		"/projects/python-app": true,
	}

	deps := &Deps{
		FS:       mockFS,
		Git:      mockGit,
		Prompter: nil,
		Logger:   mockLogger,
		Clock:    func() time.Time { return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	// Set up config
	configPath := "/home/user/.config/wsm/config.json"
	configData := `{
		"workspace_dir": "/home/user/workspaces",
		"registry_path": "/home/user/.config/wsm/registry.json"
	}`
	if err := mockFS.WriteFile(configPath, []byte(configData), 0644); err != nil {
		panic(err)
	}

	service := NewWorkspaceService(deps)

	// Act: Discover repositories
	err := service.DiscoverRepositories(context.Background(), []string{"/projects"}, true, 2)
	if err != nil {
		t.Fatalf("Failed to discover repositories: %v", err)
	}

	// Assert: Check that repositories were discovered and saved
	repos, err := service.ListRepositories()
	if err != nil {
		t.Fatalf("Failed to list repositories: %v", err)
	}

	if len(repos) < 3 {
		t.Errorf("Expected at least 3 discovered repositories, got %d", len(repos))
	}

	// Check that categories were detected correctly
	categoryMap := make(map[string][]string)
	for _, repo := range repos {
		categoryMap[repo.Name] = repo.Categories
	}

	if !contains(categoryMap["go-app"], "go") {
		t.Errorf("Expected go-app to have 'go' category")
	}

	if !contains(categoryMap["node-app"], "nodejs") {
		t.Errorf("Expected node-app to have 'nodejs' category")
	}

	if !contains(categoryMap["python-app"], "python") {
		t.Errorf("Expected python-app to have 'python' category")
	}
}

// Helper function for tests
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
