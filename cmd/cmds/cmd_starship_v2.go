package cmds

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/ux"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func NewStarshipCommandV2() *cobra.Command {
	var (
		symbol   string
		style    string
		showDate bool
		force    bool
	)

	cmd := &cobra.Command{
		Use:   "starship-v2",
		Short: "Generate starship configuration for workspace display using the new service architecture",
		Long: `Generate a starship configuration snippet that displays the current workspace name
in your shell prompt when inside a workspace directory using the new service architecture.

The new architecture provides:
- Better path detection and configuration file handling
- Structured logging for configuration operations
- Improved error handling for file system operations
- Clean separation between config generation and file operations

The configuration adds a custom module that:
- Detects when you're in a workspace directory (matching /workspaces/YYYY-MM-DD/)
- Extracts and displays the workspace name
- Optionally shows the date as well

Examples:
  # Generate default configuration
  wsm starship-v2

  # Customize the symbol and color
  wsm starship-v2 --symbol "⚡" --style "bold fg:#00ff00"

  # Include date in the display
  wsm starship-v2 --show-date

  # Force append without confirmation
  wsm starship-v2 --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStarshipCommandV2(cmd.Context(), symbol, style, showDate, force)
		},
	}

	cmd.Flags().StringVar(&symbol, "symbol", "🔧 ", "Symbol to display in the prompt")
	cmd.Flags().StringVar(&style, "style", "bold fg:#ff79c6", "Style for the prompt segment")
	cmd.Flags().BoolVar(&showDate, "show-date", false, "Include the date in the workspace display")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force append to starship config without confirmation")

	return cmd
}

func runStarshipCommandV2(ctx context.Context, symbol, style string, showDate, force bool) error {
	// Initialize the new service architecture
	deps := service.NewDeps()
	workspaceService := service.NewWorkspaceService(deps)

	deps.Logger.Info("Generating starship configuration",
		ux.Field("symbol", symbol),
		ux.Field("style", style),
		ux.Field("show_date", showDate),
		ux.Field("force", force))

	// Generate the configuration using the service
	starshipReq := service.StarshipRequest{
		Symbol:   symbol,
		Style:    style,
		ShowDate: showDate,
		Force:    force,
	}

	response, err := workspaceService.GenerateStarshipConfig(ctx, starshipReq)
	if err != nil {
		return errors.Wrap(err, "failed to generate starship configuration")
	}

	// Print the configuration
	fmt.Println("Generated starship configuration:")
	fmt.Println()
	fmt.Println(response.Config)
	fmt.Println()

	// Ask for confirmation unless forced
	if !force {
		var confirmed bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Append this configuration to %s?", response.ConfigPath)).
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

	// Apply configuration using the service
	err = workspaceService.ApplyStarshipConfig(ctx, response.ConfigPath, response.Config)
	if err != nil {
		return errors.Wrap(err, "failed to apply starship configuration")
	}

	fmt.Printf("✓ Configuration appended to %s\n", response.ConfigPath)
	return nil
}
