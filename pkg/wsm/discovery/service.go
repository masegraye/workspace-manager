package discovery

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/config"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/fs"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/git"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
)

// Service handles repository discovery operations
type Service struct {
	fs     fs.FileSystem
	git    git.Client
	logger ux.Logger
	config *config.Service
}

// New creates a new discovery service
func New(fileSystem fs.FileSystem, gitClient git.Client, logger ux.Logger, configService *config.Service) *Service {
	return &Service{
		fs:     fileSystem,
		git:    gitClient,
		logger: logger,
		config: configService,
	}
}

// DiscoverOptions contains options for repository discovery
type DiscoverOptions struct {
	Recursive bool
	MaxDepth  int
	Paths     []string
}

// Discover discovers git repositories in the specified paths
func (s *Service) Discover(ctx context.Context, opts DiscoverOptions) ([]domain.Repository, error) {
	s.logger.Info("Starting repository discovery", 
		ux.Field("paths", len(opts.Paths)),
		ux.Field("recursive", opts.Recursive),
		ux.Field("maxDepth", opts.MaxDepth))

	var allRepos []domain.Repository

	for _, path := range opts.Paths {
		repos, err := s.scanDirectory(ctx, path, opts.Recursive, opts.MaxDepth, 0)
		if err != nil {
			s.logger.Error("Failed to scan directory", 
				ux.Field("path", path),
				ux.Field("error", err))
			return nil, errors.Wrapf(err, "failed to scan directory %s", path)
		}
		allRepos = append(allRepos, repos...)
	}

	s.logger.Info("Discovery completed", 
		ux.Field("repositories", len(allRepos)))

	return allRepos, nil
}

// FindRepositories finds repositories by name from the registry
func (s *Service) FindRepositories(repoNames []string) ([]domain.Repository, error) {
	registry, err := s.config.LoadRegistry()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load registry")
	}

	var found []domain.Repository
	var missing []string

	for _, name := range repoNames {
		repo := s.findRepositoryByName(registry.Repositories, name)
		if repo != nil {
			found = append(found, *repo)
		} else {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return nil, errors.Errorf("repositories not found: %s", strings.Join(missing, ", "))
	}

	return found, nil
}

// UpdateRegistry updates the repository registry with discovered repositories
func (s *Service) UpdateRegistry(repositories []domain.Repository) error {
	registry, err := s.config.LoadRegistry()
	if err != nil {
		return errors.Wrap(err, "failed to load registry")
	}

	// Merge with existing repositories
	registry.Repositories = s.mergeRepositories(registry.Repositories, repositories)
	registry.LastScan = time.Now()

	if err := s.config.SaveRegistry(registry); err != nil {
		return errors.Wrap(err, "failed to save registry")
	}

	s.logger.Info("Registry updated", 
		ux.Field("repositories", len(registry.Repositories)))

	return nil
}

// GetRepositories returns all repositories from the registry
func (s *Service) GetRepositories() ([]domain.Repository, error) {
	registry, err := s.config.LoadRegistry()
	if err != nil {
		return nil, errors.Wrap(err, "failed to load registry")
	}

	return registry.Repositories, nil
}

// GetRepositoriesByTags returns repositories filtered by tags
func (s *Service) GetRepositoriesByTags(tags []string) ([]domain.Repository, error) {
	allRepos, err := s.GetRepositories()
	if err != nil {
		return nil, err
	}

	if len(tags) == 0 {
		return allRepos, nil
	}

	var filtered []domain.Repository
	for _, repo := range allRepos {
		if s.hasAllTags(repo.Categories, tags) {
			filtered = append(filtered, repo)
		}
	}

	return filtered, nil
}

// hasAllTags checks if repository has all the specified tags
func (s *Service) hasAllTags(repoTags, requiredTags []string) bool {
	for _, required := range requiredTags {
		found := false
		for _, tag := range repoTags {
			if strings.EqualFold(tag, required) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// scanDirectory recursively scans a directory for git repositories
func (s *Service) scanDirectory(ctx context.Context, path string, recursive bool, maxDepth, currentDepth int) ([]domain.Repository, error) {
	if recursive && maxDepth > 0 && currentDepth >= maxDepth {
		return nil, nil
	}

	var repositories []domain.Repository

	// Check if this directory is a git repository
	isRepo, err := s.git.IsRepository(ctx, path)
	if err != nil {
		s.logger.Debug("Failed to check if directory is repository", 
			ux.Field("path", path),
			ux.Field("error", err))
	} else if isRepo {
		repo, err := s.analyzeRepository(ctx, path)
		if err != nil {
			s.logger.Warn("Failed to analyze repository", 
				ux.Field("path", path),
				ux.Field("error", err))
		} else {
			repositories = append(repositories, *repo)
			s.logger.Debug("Found repository", 
				ux.Field("name", repo.Name),
				ux.Field("path", repo.Path))
		}
	}

	// If recursive, scan subdirectories
	if recursive {
		entries, err := s.fs.ReadDir(path)
		if err != nil {
			s.logger.Debug("Failed to read directory", 
				ux.Field("path", path),
				ux.Field("error", err))
			return repositories, nil
		}

		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				subPath := s.fs.Join(path, entry.Name())
				subRepos, err := s.scanDirectory(ctx, subPath, recursive, maxDepth, currentDepth+1)
				if err != nil {
					s.logger.Debug("Failed to scan subdirectory", 
						ux.Field("path", subPath),
						ux.Field("error", err))
					continue
				}
				repositories = append(repositories, subRepos...)
			}
		}
	}

	return repositories, nil
}

// analyzeRepository analyzes a git repository and extracts metadata
func (s *Service) analyzeRepository(ctx context.Context, path string) (*domain.Repository, error) {
	name := filepath.Base(path)
	
	// Get remote URL
	remoteURL, err := s.git.RemoteURL(ctx, path)
	if err != nil {
		s.logger.Debug("Failed to get remote URL", 
			ux.Field("path", path),
			ux.Field("error", err))
		remoteURL = ""
	}

	// Get current branch
	currentBranch, err := s.git.CurrentBranch(ctx, path)
	if err != nil {
		s.logger.Debug("Failed to get current branch", 
			ux.Field("path", path),
			ux.Field("error", err))
		currentBranch = ""
	}

	// Get branches
	branches, err := s.git.Branches(ctx, path)
	if err != nil {
		s.logger.Debug("Failed to get branches", 
			ux.Field("path", path),
			ux.Field("error", err))
		branches = []string{}
	}

	// Get tags
	tags, err := s.git.Tags(ctx, path)
	if err != nil {
		s.logger.Debug("Failed to get tags", 
			ux.Field("path", path),
			ux.Field("error", err))
		tags = []string{}
	}

	// Get last commit
	lastCommit, err := s.git.LastCommit(ctx, path)
	if err != nil {
		s.logger.Debug("Failed to get last commit", 
			ux.Field("path", path),
			ux.Field("error", err))
		lastCommit = ""
	}

	// Detect categories
	categories := s.detectCategories(path)

	return &domain.Repository{
		Name:          name,
		Path:          path,
		RemoteURL:     remoteURL,
		CurrentBranch: currentBranch,
		Branches:      branches,
		Tags:          tags,
		LastCommit:    lastCommit,
		LastUpdated:   time.Now(),
		Categories:    categories,
	}, nil
}

// detectCategories detects what type of project this repository is
func (s *Service) detectCategories(path string) []string {
	var categories []string

	// Check for Go project
	if s.fs.Exists(s.fs.Join(path, "go.mod")) {
		categories = append(categories, "go")
	}

	// Check for Node.js project
	if s.fs.Exists(s.fs.Join(path, "package.json")) {
		categories = append(categories, "nodejs")
	}

	// Check for Python project
	if s.fs.Exists(s.fs.Join(path, "setup.py")) || 
	   s.fs.Exists(s.fs.Join(path, "pyproject.toml")) ||
	   s.fs.Exists(s.fs.Join(path, "requirements.txt")) {
		categories = append(categories, "python")
	}

	// Check for Rust project
	if s.fs.Exists(s.fs.Join(path, "Cargo.toml")) {
		categories = append(categories, "rust")
	}

	// Check for Java project
	if s.fs.Exists(s.fs.Join(path, "pom.xml")) || 
	   s.fs.Exists(s.fs.Join(path, "build.gradle")) {
		categories = append(categories, "java")
	}

	// Check for Docker
	if s.fs.Exists(s.fs.Join(path, "Dockerfile")) || 
	   s.fs.Exists(s.fs.Join(path, "docker-compose.yml")) {
		categories = append(categories, "docker")
	}

	// Check for web project
	if s.fs.Exists(s.fs.Join(path, "index.html")) {
		categories = append(categories, "web")
	}

	return categories
}

// findRepositoryByName finds a repository by name in the list
func (s *Service) findRepositoryByName(repositories []domain.Repository, name string) *domain.Repository {
	for _, repo := range repositories {
		if repo.Name == name {
			return &repo
		}
	}
	return nil
}

// mergeRepositories merges new repositories with existing ones, updating existing entries
func (s *Service) mergeRepositories(existing, new []domain.Repository) []domain.Repository {
	repoMap := make(map[string]domain.Repository)

	// Add existing repositories to map
	for _, repo := range existing {
		repoMap[repo.Path] = repo
	}

	// Update with new repositories
	for _, repo := range new {
		repoMap[repo.Path] = repo
	}

	// Convert back to slice
	var merged []domain.Repository
	for _, repo := range repoMap {
		merged = append(merged, repo)
	}

	return merged
}
