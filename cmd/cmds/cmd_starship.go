package cmds

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewStarshipCommand creates the starship command
func NewStarshipCommand() *cobra.Command {
	var symbol string
	var style string
	var showDate bool
	var force bool

	cmd := &cobra.Command{
		Use:   "starship",
		Short: "Generate starship configuration for workspace display",
		Long: `Generate a starship configuration snippet that displays the current workspace name
in your shell prompt when inside a workspace directory.

The configuration adds a custom module that:
- Detects when you're in a workspace directory (matching /workspaces/YYYY-MM-DD/)
- Extracts and displays the workspace name
- Optionally shows the date as well

Examples:
  # Generate default configuration
  workspace-manager starship

  # Customize the symbol and color
  workspace-manager starship --symbol "⚡" --style "bold fg:#00ff00"

  # Include date in the display
  workspace-manager starship --show-date

  # Force append without confirmation
  workspace-manager starship --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStarshipCommand(symbol, style, showDate, force)
		},
	}

	cmd.Flags().StringVar(&symbol, "symbol", "🔧 ", "Symbol to display in the prompt")
	cmd.Flags().StringVar(&style, "style", "bold fg:#ff79c6", "Style for the prompt segment")
	cmd.Flags().BoolVar(&showDate, "show-date", false, "Include the date in the workspace display")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force append to starship config without confirmation")

	return cmd
}

func runStarshipCommand(symbol, style string, showDate, force bool) error {
	// Generate the configuration
	config := generateStarshipConfig(symbol, style, showDate)

	// Print the configuration
	fmt.Println("Generated starship configuration:")
	fmt.Println()
	fmt.Println(config)
	fmt.Println()

	// Get the starship config path
	configPath, err := getStarshipConfigPath()
	if err != nil {
		return errors.Wrap(err, "failed to determine starship config path")
	}

	// Ask for confirmation unless forced
	if !force {
		var confirmed bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Append this configuration to %s?", configPath)).
					Description("This will add the workspace module to your starship configuration.").
					Value(&confirmed),
			),
		)

		err := form.Run()
		if err != nil {
			// Check if user cancelled/aborted the form
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "user aborted") ||
				strings.Contains(errMsg, "cancelled") ||
				strings.Contains(errMsg, "aborted") {
				fmt.Println("Configuration not added.")
				return nil
			}
			return errors.Wrap(err, "failed to get user confirmation")
		}

		if !confirmed {
			fmt.Println("Configuration not added.")
			return nil
		}
	}

	// Append to the config file
	err = appendToStarshipConfig(configPath, config)
	if err != nil {
		return errors.Wrap(err, "failed to append configuration")
	}

	fmt.Printf("✓ Configuration appended to %s\n", configPath)
	return nil
}

func generateStarshipConfig(symbol, style string, showDate bool) string {
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

func getStarshipConfigPath() (string, error) {
	var configPath string

	if runtime.GOOS == "darwin" {
		// macOS
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configPath = filepath.Join(homeDir, ".config", "starship.toml")
	} else {
		// Linux and others
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			configPath = filepath.Join(xdgConfig, "starship.toml")
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configPath = filepath.Join(homeDir, ".config", "starship.toml")
		}
	}

	return configPath, nil
}

func appendToStarshipConfig(configPath, config string) error {
	// Create the config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	// Open file for appending, create if it doesn't exist
	file, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to open config file")
	}
	defer func() { _ = file.Close() }()

	// Add a newline before the config if the file exists and is not empty
	stat, err := file.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to get file stats")
	}

	if stat.Size() > 0 {
		_, err = file.WriteString("\n\n")
		if err != nil {
			return errors.Wrap(err, "failed to write newline")
		}
	}

	// Write the configuration
	_, err = file.WriteString(config + "\n")
	if err != nil {
		return errors.Wrap(err, "failed to write configuration")
	}

	return nil
}
