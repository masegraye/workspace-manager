package metadata

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
)

// Builder creates workspace metadata
type Builder struct {
	clock func() time.Time
}

// New creates a new metadata builder
func New(clock func() time.Time) *Builder {
	return &Builder{clock: clock}
}

// WorkspaceMetadata represents the structure saved to .wsm/wsm.json
type WorkspaceMetadata struct {
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Branch       string               `json:"branch"`
	BaseBranch   string               `json:"base_branch"`
	GoWorkspace  bool                 `json:"go_workspace"`
	AgentMD      string               `json:"agent_md"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	Repositories []RepositoryMetadata `json:"repositories"`
	Environment  map[string]string    `json:"environment"`
}

// RepositoryMetadata represents repository information in workspace metadata
type RepositoryMetadata struct {
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Categories   []string `json:"categories"`
	WorktreePath string   `json:"worktree_path"`
}

// BuildWorkspaceMetadata creates the metadata JSON for a workspace
func (b *Builder) BuildWorkspaceMetadata(ws domain.Workspace) ([]byte, error) {
	now := b.clock()

	metadata := WorkspaceMetadata{
		Name:        ws.Name,
		Path:        ws.Path,
		Branch:      ws.Branch,
		BaseBranch:  ws.BaseBranch,
		GoWorkspace: ws.GoWorkspace,
		AgentMD:     ws.AgentMD,
		CreatedAt:   ws.Created,
		UpdatedAt:   now,
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

// ParseWorkspaceMetadata parses workspace metadata from JSON
func (b *Builder) ParseWorkspaceMetadata(data []byte) (*WorkspaceMetadata, error) {
	var metadata WorkspaceMetadata
	err := json.Unmarshal(data, &metadata)
	return &metadata, err
}
