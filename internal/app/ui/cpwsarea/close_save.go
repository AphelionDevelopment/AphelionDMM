// APHELION EDIT ADDITION START - ATOMIC_SAVE
package cpwsarea

import "sdmm/internal/app/ui/cpwsarea/workspace"

func saveWorkspacesBeforeClose(workspaces []*workspace.Workspace, callback func(bool)) bool {
	for _, ws := range workspaces {
		if !ws.Save() {
			if callback != nil {
				callback(false)
			}
			return false
		}
	}
	return true
}

// APHELION EDIT ADDITION END
