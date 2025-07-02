package cmds

import (
	"github.com/carapace-sh/carapace"
	"github.com/go-go-golems/workspace-manager/pkg/wsm/service"
)

// WorkspaceNameCompletion returns a carapace.Action that completes workspace names.
func WorkspaceNameCompletion() carapace.Action {
	return carapace.ActionCallback(func(ctx carapace.Context) carapace.Action {
		deps := service.NewDeps()
		workspaceService := service.NewWorkspaceService(deps)
		workspaces, err := workspaceService.ListWorkspaces()
		if err != nil {
			return carapace.ActionMessage("failed to load workspaces")
		}
		var names []string
		for _, ws := range workspaces {
			names = append(names, ws.Name)
		}
		return carapace.ActionValues(names...)
	})
}

// RepositoryNameCompletion returns a carapace.Action that completes repository names
// from the registry for add commands.
func RepositoryNameCompletion() carapace.Action {
	return carapace.ActionCallback(func(ctx carapace.Context) carapace.Action {
		// TODO: Implement repository completion with new service architecture
		return carapace.ActionMessage("repository completion not yet implemented")
	})
}

// WorkspaceRepositoryCompletion returns a carapace.Action that completes repository names
// that are currently part of the specified workspace (for remove commands).
func WorkspaceRepositoryCompletion() carapace.Action {
	return carapace.ActionCallback(func(ctx carapace.Context) carapace.Action {
		if len(ctx.Args) < 1 {
			return carapace.ActionMessage("workspace name required")
		}
		workspaceName := ctx.Args[0]

		deps := service.NewDeps()
		workspaceService := service.NewWorkspaceService(deps)
		workspaces, err := workspaceService.ListWorkspaces()
		if err != nil {
			return carapace.ActionMessage("failed to load workspaces")
		}

		for _, ws := range workspaces {
			if ws.Name == workspaceName {
				var names []string
				for _, repo := range ws.Repositories {
					names = append(names, repo.Name)
				}
				return carapace.ActionValues(names...)
			}
		}
		return carapace.ActionMessage("workspace not found")
	})
}

// TagCompletion returns a carapace.Action that completes repository tags.
func TagCompletion() carapace.Action {
	return carapace.ActionCallback(func(ctx carapace.Context) carapace.Action {
		// TODO: Implement tag completion with new service architecture
		return carapace.ActionMessage("tag completion not yet implemented")
	})
}


