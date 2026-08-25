package wsmap

import (
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
	if err := dmmsave.Save(ws.app.LoadedEnvironment(), ws.paneMap.Dmm(), dmmsave.Config{
		Format:            saveFormat,
		SanitizeVariables: editorPrefs.SanitizeVariables,
	}); err != nil {
		log.Error().Err(err).Msg("unable to save map workspace")
		util.ShowErrorDialog("Unable to save the map: " + err.Error())
		return false
	}

	ws.app.CommandStorage().ForceBalance(ws.CommandStackId())
	// APHELION EDIT ADDITION END
	return true
}
