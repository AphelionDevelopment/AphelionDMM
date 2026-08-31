# StrongDMM upstream drift review

Date: 2026-08-31

## Source state

- Downstream branch: `main`
- Downstream revision: `c3469c60d5cd39c21268e14ee38643910601b2e9`
- Upstream remote: `https://github.com/SpaiR/StrongDMM.git`
- Command: `git fetch upstream main --prune`
- Fetched revision: `5241698aeca9e83fd61048d835e9318d3610c16a`
- Fetched commit date: `2026-05-08T14:46:41+03:00`
- Fetched subject: `[ci skip] v2.18.0.alpha (manifest)`
- Divergence at committed `HEAD`: downstream is seven commits ahead and zero commits behind.
- Working tree: dirty before and after the fetch; no checkout, merge, rebase, reset, commit, or push was performed.

The fresh fetch resolved to the same revision used by the 2026-08-30 audit. There is no new upstream delta to reconcile. The result is current as of this fetch, not a stale local-remote observation.

## Audited ownership files

All 16 implementation files named by ADMM-AUDIT-007 are classified `marked conflict, no upstream drift`: they diverge downstream, every Aphelion-owned implementation span is now bounded by `APHELION EDIT` markers, and upstream made no change after the audit baseline.

- `internal/app/command/command.go`
- `internal/app/command/storage.go`
- `internal/app/ui/cpprefabs/menu.go`
- `internal/app/ui/cpsearch/process.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/psettings/map_size.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/psettings/psettings.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/tilemenu/process.go`
- `internal/app/ui/cpwsarea/wsmap/tools/add.go`
- `internal/app/ui/cpwsarea/wsmap/tools/delete.go`
- `internal/app/ui/cpwsarea/wsmap/tools/fill.go`
- `internal/app/ui/cpwsarea/wsmap/tools/grab.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/collaboration_presence.go`
- `internal/dmapi/dmmap/dmmdata/save_atomic.go`
- `internal/dmapi/dmmap/dmmdata/save_atomic_other.go`
- `internal/dmapi/dmmap/dmmdata/save_atomic_windows.go`

Focused tests for every affected package passed after the marker pass. The collaboration and atomic-save files remain thin inherited-package adapters because they require package-private editor and replacement primitives; moving them would require exporting inherited internals.

## Other divergent inherited implementation files

The following 47 implementation files are also classified `marked conflict, no upstream drift`. They already contained ownership markers before this remediation and were not rewritten:

- `internal/app/action.go`
- `internal/app/action_user.go`
- `internal/app/app.go`
- `internal/app/config/config.go`
- `internal/app/config_preference.go`
- `internal/app/project.go`
- `internal/app/selfupdate/manifest.go`
- `internal/app/selfupdate/selfupdate.go`
- `internal/app/ui/cpvareditor/vareditor.go`
- `internal/app/ui/cpwsarea/wsarea.go`
- `internal/app/ui/cpwsarea/wscreatemap/save.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/editor/commit.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/editor/editor.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/editor/instance.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/editor/tile.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/pmap.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/pquickedit/pquickedit.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/psettings/screenshot.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/tilemenu/tilemenu.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/tools.go`
- `internal/app/ui/cpwsarea/wsmap/save.go`
- `internal/app/ui/cpwsarea/wsmap/tools/move.go`
- `internal/app/ui/cpwsarea/wsmap/tools/replace.go`
- `internal/app/ui/cpwsarea/wsmap/tools/tool.go`
- `internal/app/ui/cpwsarea/wsmap/tools/tools.go`
- `internal/app/ui/layout/config.go`
- `internal/app/ui/layout/layout.go`
- `internal/app/ui/layout/lnode/lnode.go`
- `internal/app/ui/menu/menu.go`
- `internal/app/update.go`
- `internal/app/window/process.go`
- `internal/app/window/util.go`
- `internal/dmapi/dmicon/dmi.go`
- `internal/dmapi/dmmap/dmm.go`
- `internal/dmapi/dmmap/dmmdata/dmmdata.go`
- `internal/dmapi/dmmap/dmmdata/parse.go`
- `internal/dmapi/dmmap/dmmdata/save_dm.go`
- `internal/dmapi/dmmap/dmmdata/save_tgm.go`
- `internal/dmapi/dmmap/dmminstance/instance.go`
- `internal/dmapi/dmmsave/dmmsave.go`
- `internal/dmapi/dmmsave/save_process.go`
- `internal/platform/gl.go`
- `internal/req/req.go`
- `third_party/sdmmparser/sdmmparser.go`
- `third_party/sdmmparser/src/environment.rs`
- `third_party/sdmmparser/src/icon.rs`
- `third_party/sdmmparser/src/lib.rs`

## Manual semantic review

Nine downstream-only test files under inherited package trees contain no ownership markers. They are classified `manual semantic review, no upstream drift`; markers are not added to test-only files, but their assertions must be reviewed during a future reconciliation:

- `internal/app/command/storage_async_test.go`
- `internal/app/selfupdate/selfupdate_test.go`
- `internal/app/ui/cpwsarea/wsarea_test.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/collaboration_presence_test.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/editor/collaboration_test.go`
- `internal/app/ui/cpwsarea/wsmap/pmap/psettings/map_size_test.go`
- `internal/app/window/util_test.go`
- `internal/dmapi/dmmap/dmmdata/save_atomic_test.go`
- `internal/req/req_test.go`

## Reconciliation decision

No reconciliation is required because the fetched upstream revision did not advance. Future upstream movement must review every marked span above, with parser, serialization, updater, atomic-save, editor mutation, undo, and collaboration paths treated as high risk under `docs/agent/upstream-drift.md`.
