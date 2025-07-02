package config

import (
	"encoding/json"
	"path/filepath"

	"github.com/go-go-golems/workspace-manager/pkg/wsm/domain"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/fs"
	"github.com/pkg/errors"
)

// Service handles configuration management
type Service struct {
	fs fs.FileSystem
}

// New creates a new config service
func New(fileSystem fs.FileSystem) *Service {
	return &Service{fs: fileSystem}
}

// Load loads the workspace configuration
func (s *Service) Load() (*domain.WorkspaceConfig, error) {
	configDir, err := s.fs.UserConfigDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user config directory")
	}
	
	configPath := s.fs.Join(configDir, "wsm", "config.json")
	
	if !s.fs.Exists(configPath) {
		// Return default configuration if config file doesn't exist
		homeDir, err := s.fs.UserHomeDir()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get user home directory")
		}
		
		return &domain.WorkspaceConfig{
			WorkspaceDir: s.fs.Join(homeDir, "workspaces"),
			TemplateDir:  s.fs.Join(configDir, "wsm", "templates"),
			RegistryPath: s.fs.Join(configDir, "wsm", "registry.json"),
		}, nil
	}
	
	data, err := s.fs.ReadFile(configPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}
	
	var config domain.WorkspaceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}
	
	return &config, nil
}

// Save saves the workspace configuration
func (s *Service) Save(config *domain.WorkspaceConfig) error {
	configDir, err := s.fs.UserConfigDir()
	if err != nil {
		return errors.Wrap(err, "failed to get user config directory")
	}
	
	configPath := s.fs.Join(configDir, "wsm", "config.json")
	configDirPath := filepath.Dir(configPath)
	
	if err := s.fs.MkdirAll(configDirPath, 0755); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal config")
	}
	
	if err := s.fs.WriteFile(configPath, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	
	return nil
}

// LoadRegistry loads the repository registry
func (s *Service) LoadRegistry() (*domain.RepositoryRegistry, error) {
	config, err := s.Load()
	if err != nil {
		return nil, err
	}
	
	if !s.fs.Exists(config.RegistryPath) {
		// Return empty registry if file doesn't exist
		return &domain.RepositoryRegistry{
			Repositories: []domain.Repository{},
		}, nil
	}
	
	data, err := s.fs.ReadFile(config.RegistryPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read registry file")
	}
	
	var registry domain.RepositoryRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, errors.Wrap(err, "failed to parse registry file")
	}
	
	return &registry, nil
}

// SaveRegistry saves the repository registry
func (s *Service) SaveRegistry(registry *domain.RepositoryRegistry) error {
	config, err := s.Load()
	if err != nil {
		return err
	}
	
	registryDir := filepath.Dir(config.RegistryPath)
	if err := s.fs.MkdirAll(registryDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create registry directory")
	}
	
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal registry")
	}
	
	if err := s.fs.WriteFile(config.RegistryPath, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write registry file")
	}
	
	return nil
}

// SaveWorkspace saves a workspace metadata file
func (s *Service) SaveWorkspace(workspace *domain.Workspace) error {
	metadataDir := filepath.Dir(workspace.MetadataPath())
	if err := s.fs.MkdirAll(metadataDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create workspace metadata directory")
	}
	
	// Note: This is a simplified version. In practice, you'd use the metadata.Builder
	data, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal workspace")
	}
	
	if err := s.fs.WriteFile(workspace.MetadataPath(), data, 0644); err != nil {
		return errors.Wrap(err, "failed to write workspace metadata")
	}
	
	return nil
}
