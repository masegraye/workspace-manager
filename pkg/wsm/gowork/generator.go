package gowork

import (
	"fmt"
	"strings"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
)

// Generator creates go.work files for workspaces
type Generator struct{}

// New creates a new go.work generator
func New() *Generator {
	return &Generator{}
}

// Generate creates the content for a go.work file based on the workspace repositories
func (g *Generator) Generate(workspace domain.Workspace) string {
	var content strings.Builder
	content.WriteString("go 1.21\n\n")
	content.WriteString("use (\n")
	
	for _, repo := range workspace.Repositories {
		if repo.IsGoProject() {
			content.WriteString(fmt.Sprintf("    ./%s\n", repo.Name))
		}
	}
	
	content.WriteString(")\n")
	return content.String()
}

// GenerateFromRepositories creates go.work content from a list of repositories
func (g *Generator) GenerateFromRepositories(repositories []domain.Repository) string {
	var content strings.Builder
	content.WriteString("go 1.21\n\n")
	content.WriteString("use (\n")
	
	for _, repo := range repositories {
		if repo.IsGoProject() {
			content.WriteString(fmt.Sprintf("    ./%s\n", repo.Name))
		}
	}
	
	content.WriteString(")\n")
	return content.String()
}

// ShouldGenerate returns true if any repository in the workspace is a Go project
func (g *Generator) ShouldGenerate(workspace domain.Workspace) bool {
	return workspace.NeedsGoWorkspace()
}
