package cpvareditor

import (
	"sdmm/internal/app/config"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"testing"
)

type auditUnknownApp struct {
	App
	environment   *dmenv.Dme
	configuration *vareditorConfig
}

func (app *auditUnknownApp) LoadedEnvironment() *dmenv.Dme   { return app.environment }
func (app *auditUnknownApp) ConfigFind(string) config.Config { return app.configuration }

func TestAuditVariableEditorAcceptsUnknownPrefab(t *testing.T) {
	app := &auditUnknownApp{environment: &dmenv.Dme{Objects: map[string]*dmenv.Object{}}, configuration: &vareditorConfig{Version: configVersion}}
	variables := &dmvars.MutableVariables{}
	variables.Put("custom_value", "7")
	prefab := dmmprefab.New(dmmprefab.IdNone, "/obj/audit_unknown", variables.ToImmutable())
	editor := &VarEditor{app: app}
	defer func() {
		if failure := recover(); failure != nil {
			t.Errorf("opening preserved unknown prefab panicked: %v", failure)
		}
	}()
	editor.EditPrefab(prefab)
	if editor.isFilteredVariable("custom_value") {
		t.Fatal("unknown explicit variable was filtered out")
	}
	if !editor.isReadOnly("custom_value") {
		t.Fatal("missing metadata did not disable metadata-dependent editing")
	}
	if editor.initialVarValue("custom_value") != "7" {
		t.Fatal("unknown default replaced the explicit value")
	}
	app.configuration.ShowModified = true
	if editor.isFilteredVariable("custom_value") {
		t.Fatal("modified-only view hid unknown explicit data")
	}
	if len(editor.variablesPaths) != 1 || len(editor.variablesNamesByPaths[prefab.Path()]) != 1 {
		t.Fatal("grouped view lost unknown explicit data")
	}
	editor.setPrefabVariable("custom_value", "8")
	if editor.prefab != prefab || editor.prefab.Vars().ValueV("custom_value", "") != "7" {
		t.Fatal("unknown prefab contents changed")
	}
}
