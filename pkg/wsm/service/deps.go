package service

import (
	"time"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/fs"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/git"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
)

// Deps contains all external dependencies for services
type Deps struct {
	FS       fs.FileSystem
	Git      git.Client
	Prompter ux.Prompter
	Logger   ux.Logger
	Clock    func() time.Time
}

// NewDeps creates a new dependencies container with production implementations
func NewDeps() *Deps {
	huhPrompter := ux.NewHuhPrompter()
	return &Deps{
		FS:       fs.NewOSFileSystem(),
		Git:      git.NewExecClient(),
		Prompter: huhPrompter, // HuhPrompter implements both Prompter and MultiSelectPrompter
		Logger:   ux.NewZerologLogger(),
		Clock:    time.Now,
	}
}

// NewTestDeps creates dependencies suitable for testing
func NewTestDeps(fs fs.FileSystem, git git.Client, prompter ux.Prompter, logger ux.Logger) *Deps {
	return &Deps{
		FS:       fs,
		Git:      git,
		Prompter: prompter,
		Logger:   logger,
		Clock:    func() time.Time { return time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
}
