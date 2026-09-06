package wsmap

import (
	// APHELION EDIT ADDITION START - ATOMIC_SAVE
	"context"
	"sdmm/internal/aphelion/collab/mapadapter"
	"sdmm/internal/dmapi/dmmap"
	// APHELION EDIT ADDITION END

	"sdmm/internal/app/prefs"
	"sdmm/internal/dmapi/dmmsave"
	"sdmm/internal/util"

	"github.com/rs/zerolog/log"
)

func (ws *WsMap) Save() bool {
	log.Print("saving map workspace:", ws.CommandStackId())

	editorPrefs := ws.app.Prefs().Editor

	var saveFormat dmmsave.Format
	switch editorPrefs.SaveFormat {
	case prefs.SaveFormatInitial:
		saveFormat = dmmsave.FormatInitial
	case prefs.SaveFormatTGM:
		saveFormat = dmmsave.FormatTGM
	case prefs.SaveFormatDMM:
		saveFormat = dmmsave.FormatDM
	}

	// APHELION EDIT ADDITION START - ATOMIC_SAVE
	snapshot, err := ws.paneMap.Editor().SaveSnapshot(context.Background())
	if err != nil {
		return ws.saveFailed(err)
	}
	source := ws.paneMap.Dmm()
	acknowledged := &dmmap.Dmm{Name: source.Name, Path: source.Path, Backup: source.Backup}
	if err := mapadapter.ApplyWithEnvironment(acknowledged, snapshot, ws.app.LoadedEnvironment()); err != nil {
		return ws.saveFailed(err)
	}
	if err := dmmsave.Save(ws.app.LoadedEnvironment(), acknowledged, dmmsave.Config{
		Format:            saveFormat,
		SanitizeVariables: editorPrefs.SanitizeVariables,
	}); err != nil {
		return ws.saveFailed(err)
	}

	ws.savedMapHash, _ = snapshot.Hash() // SaveSnapshot and ApplyWithEnvironment validated this state.
	ws.savedGeneration, ws.savedRevision = ws.paneMap.Editor().SaveVersion()
	ws.app.CommandStorage().ForceBalance(ws.CommandStackId())
	// APHELION EDIT ADDITION END
	return true
}

// APHELION EDIT ADDITION START - ATOMIC_SAVE
func (ws *WsMap) saveFailed(err error) bool {
	log.Error().Err(err).Msg("unable to save map workspace")
	ws.app.RunLater(func() { util.ShowErrorDialog("Unable to save the map: " + err.Error()) })
	return false
}

// HasUnsavedChanges checks authority when a close decision is made. The tab
// label uses a cheap version check instead of hashing the map every frame.
func (ws *WsMap) HasUnsavedChanges() bool {
	if ws.app.CommandStorage().IsModified(ws.CommandStackId()) {
		return true
	}
	snapshot, err := ws.paneMap.Editor().SaveSnapshot(context.Background())
	if err != nil {
		return true
	}
	hash, err := snapshot.Hash()
	return err != nil || hash != ws.savedMapHash
}

// APHELION EDIT ADDITION END
