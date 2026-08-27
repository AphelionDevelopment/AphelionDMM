package editor

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
	"sdmm/internal/aphelion/collab/server"
	collabui "sdmm/internal/aphelion/collab/ui"
	"sdmm/internal/app/command"
	"sdmm/internal/app/prefs"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmmclip"
	"sdmm/internal/dmapi/dmmsnap"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"

	"github.com/SpaiR/imgui-go"
	"github.com/coder/websocket"
)

func TestEditorOperationUndoRedo(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
	}
	application.commands.SetStack("test")
	attached := &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}
	editor := New(application, attached, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}

	instance := mapState.Tiles[0].Instances()[2]
	changedVariables := dmvars.Set(instance.Prefab().Vars(), "dir", "4")
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), changedVariables))
	editor.CommitOperation("Change Direction")
	assertEditorDirection(t, mapState, "4")
	if !application.commands.HasUndoV("test") {
		t.Fatal("accepted operation did not create undo command")
	}

	application.commands.UndoV("test")
	assertEditorDirection(t, mapState, "2")
	application.commands.RedoV("test")
	assertEditorDirection(t, mapState, "4")
}

func TestEditorCollaborationSnapshotIsCommittedAndDetached(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
	}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}

	snapshot, err := editor.CollaborationSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read collaboration snapshot: %v", err)
	}
	if snapshot.DocumentID != editor.documentID || len(snapshot.Tiles) == 0 {
		t.Fatalf("unexpected collaboration snapshot: %+v", snapshot)
	}
	snapshot.Tiles[0].State.Prefabs[0].Path = "/mutated/test/path"
	second, err := editor.CollaborationSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read second collaboration snapshot: %v", err)
	}
	if second.Tiles[0].State.Prefabs[0].Path == "/mutated/test/path" {
		t.Fatal("collaboration snapshot aliases editor state")
	}

	editor.BeginTileChange(mapState.Tiles[0].Coord)
	if _, err := editor.CollaborationSnapshot(context.Background()); err == nil {
		t.Fatal("collaboration snapshot accepted an uncommitted edit")
	}
}

func TestEditorDetachCollaborationExecutorRestoresLocalEditing(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
	}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	remote := attachmentEditorExecutor(t, editor.authoritative, editor.actorID)
	if err := editor.AttachCollaborationExecutor(remote); err != nil {
		t.Fatal(err)
	}
	if err := editor.DetachCollaborationExecutor(context.Background()); err != nil {
		t.Fatal(err)
	}
	if editor.executor == remote {
		t.Fatal("detach retained the collaboration executor")
	}

	instance := mapState.Tiles[0].Instances()[2]
	changedVariables := dmvars.Set(instance.Prefab().Vars(), "dir", "4")
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), changedVariables))
	editor.CommitOperation("Local Change After Leave")
	assertEditorDirection(t, mapState, "4")
}

func TestEditorDetachCollaborationExecutorRefusesPendingAcknowledgement(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
	}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	remote := &pendingAttachmentEditorExecutor{Executor: attachmentEditorExecutor(t, editor.authoritative, editor.actorID)}
	if err := editor.AttachCollaborationExecutor(remote); err != nil {
		t.Fatal(err)
	}
	if err := editor.DetachCollaborationExecutor(context.Background()); err == nil {
		t.Fatal("detach accepted an operation awaiting acknowledgement")
	}
	if editor.executor != remote {
		t.Fatal("failed detach replaced the collaboration executor")
	}
}

func attachmentEditorExecutor(t *testing.T, snapshot model.Snapshot, actorID model.ActorID) executor.Executor {
	t.Helper()
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	local, err := executor.NewLocal(document, actorID)
	if err != nil {
		t.Fatal(err)
	}
	return local
}

type pendingAttachmentEditorExecutor struct {
	executor.Executor
}

func (*pendingAttachmentEditorExecutor) HasUnacknowledgedOperations() bool {
	return true
}

func TestEditorNetworkCommitWaitsForAcknowledgementBeforeAddingUndo(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
		runLater:    make(chan func(), 8),
	}
	application.commands.SetStack("test")
	attached := &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}
	editor := New(application, attached, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	deferred := newDeferredAsyncExecutor(t, editor.authoritative, editor.actorID)
	editor.executor = deferred

	instance := mapState.Tiles[0].Instances()[2]
	changedVariables := dmvars.Set(instance.Prefab().Vars(), "dir", "4")
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), changedVariables))
	editor.CommitOperation("Network Change")
	if application.commands.HasUndoV("test") {
		t.Fatal("network operation entered undo history before acknowledgement")
	}
	deferred.resolve(t)
	application.runScheduled(t)
	if !application.commands.HasUndoV("test") {
		t.Fatal("acknowledged network operation did not enter undo history")
	}

	if !application.commands.UndoAsyncV("test", nil) {
		t.Fatal("network undo did not start")
	}
	if application.commands.HasUndoV("test") || application.commands.HasRedoV("test") {
		t.Fatal("network undo moved history before acknowledgement")
	}
	deferred.resolve(t)
	application.runScheduled(t)
	if !application.commands.HasRedoV("test") {
		t.Fatal("acknowledged network undo did not enter redo history")
	}
	assertEditorDirection(t, mapState, "2")

	if !application.commands.RedoAsyncV("test", nil) {
		t.Fatal("network redo did not start")
	}
	if application.commands.HasUndoV("test") || application.commands.HasRedoV("test") {
		t.Fatal("network redo moved history before acknowledgement")
	}
	deferred.resolve(t)
	application.runScheduled(t)
	if !application.commands.HasUndoV("test") {
		t.Fatal("acknowledged network redo did not enter undo history")
	}
	assertEditorDirection(t, mapState, "4")
}

func TestEditorProcessesNetworkProjectionOnUIThread(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
		runLater:    make(chan func(), 8),
	}
	application.commands.SetStack("test")
	attached := &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}
	editor := New(application, attached, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	network := &projectionTestExecutor{
		Local:   editor.executor.(*executor.Local),
		updates: make(chan client.Projection, 1),
	}
	editor.executor = network

	acknowledged := model.CloneSnapshot(editor.authoritative)
	acknowledged.Revision++
	acknowledged.Tiles[0].State.Prefabs[2].Vars["dir"] = "4"
	network.updates <- client.NewProjection(acknowledged)

	editor.ProcessCollaborationUpdates()
	assertEditorDirection(t, mapState, "4")
	if editor.authoritative.Revision != acknowledged.Revision {
		t.Fatalf("authoritative revision = %d, want %d", editor.authoritative.Revision, acknowledged.Revision)
	}
}

func TestEditorPendingNetworkEditUsesVisibleTileAsPrecondition(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
		runLater:    make(chan func(), 8),
	}
	application.commands.SetStack("test")
	attached := &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}
	editor := New(application, attached, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	editor.executor = newDeferredAsyncExecutor(t, editor.authoritative, editor.actorID)

	instance := mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "4")))
	editor.CommitOperation("First Network Change")

	instance = mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "8")))
	coord := model.Coord{X: 1, Y: 1, Z: 1}
	before, exists := editor.pendingChanges[coord]
	if !exists {
		t.Fatal("second edit did not capture a tile precondition")
	}
	if got := before.Prefabs[2].Vars["dir"]; got != "4" {
		t.Fatalf("second edit precondition direction = %q, want visible direction 4", got)
	}
}

func TestEditorRollsBackPendingEditAfterDisconnectAndAcceptsFreshEdit(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
		runLater:    make(chan func(), 8),
	}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	firstTransport := newEditorNetworkTransport()
	network, err := client.NewNetworkExecutor(firstTransport, editor.authoritative, editor.actorID, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	editor.executor = network

	instance := mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "4")))
	editor.CommitOperation("Interrupted Network Change")
	firstTransport.next(t)
	if !network.HasUnacknowledgedOperations() {
		t.Fatal("network edit was not awaiting acknowledgement")
	}
	lost := errors.New("forced disconnect")
	network.Suspend(lost)
	application.discardScheduled(t)
	editor.ProcessCollaborationUpdates()
	assertEditorDirection(t, mapState, "2")
	if network.HasUnacknowledgedOperations() || application.commands.HasUndoV("test") {
		t.Fatal("disconnected edit remained pending or entered undo history")
	}

	secondTransport := newEditorNetworkTransport()
	if err := network.Resume(secondTransport); err != nil {
		t.Fatal(err)
	}
	instance = mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "8")))
	editor.CommitOperation("Fresh Network Change")
	submitted := secondTransport.next(t)
	decoded, err := protocol.DecodeClient(mustEditorJSON(t, submitted))
	if err != nil {
		t.Fatal(err)
	}
	operation := decoded.Payload.(*protocol.OperationSubmitPayload).Operation
	document, err := engine.NewDocument(editor.authoritative)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := document.Apply(operation, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	mapHash, err := document.Snapshot().Hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := network.Receive(protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "accepted", SessionID: "session-1", Type: protocol.ServerOperationAccepted, Payload: mustEditorJSON(t, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: mapHash})}); err != nil {
		t.Fatal(err)
	}
	application.runScheduled(t)
	editor.ProcessCollaborationUpdates()
	assertEditorDirection(t, mapState, "8")
	if !application.commands.HasUndoV("test") {
		t.Fatal("fresh acknowledged edit did not enter undo history after reconnect")
	}
}

func TestEditorReconnectsToRealServiceAfterPendingEditAndUndoesFreshEdit(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
		runLater:    make(chan func(), 8),
	}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	snapshot, err := editor.CollaborationSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := server.StartEmbedded(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = embedded.Shutdown(context.Background()) })

	blocked := &blockingEditorSessionTransport{
		SessionTransport: client.NewWebSocketTransport(client.TransportConfig{}),
		intercepted:      make(chan struct{}),
		release:          make(chan struct{}),
	}
	transportCount := 0
	reconnectTransportCreated := make(chan struct{})
	session := collabui.NewSessionClient(collabui.SessionClientConfig{NewTransport: func() collabui.SessionTransport {
		transportCount++
		if transportCount == 1 {
			return blocked
		}
		if transportCount == 2 {
			close(reconnectTransportCreated)
		}
		return client.NewWebSocketTransport(client.TransportConfig{})
	}})
	t.Cleanup(func() { _ = session.Leave(context.Background()) })
	invitation, err := session.Create(context.Background(), embedded.Endpoint(), embedded.TakeLaunchToken(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Join(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	if err := editor.AttachCollaborationExecutor(session.NetworkExecutor()); err != nil {
		t.Fatal(err)
	}

	instance := mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "4")))
	editor.CommitOperation("Interrupted Real-Service Change")
	select {
	case <-blocked.intercepted:
	case <-time.After(time.Second):
		t.Fatal("editor operation did not reach the gated WebSocket transport")
	}
	if err := blocked.Close(websocket.StatusInternalError, "forced disconnect with editor operation in flight"); err != nil {
		t.Fatal(err)
	}
	close(blocked.release)
	application.discardScheduled(t)
	select {
	case <-reconnectTransportCreated:
	case <-time.After(time.Second):
		t.Fatal("session did not create a reconnect transport")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && session.Status().State != client.StateCaughtUp {
		time.Sleep(10 * time.Millisecond)
	}
	if status := session.Status(); status.State != client.StateCaughtUp {
		t.Fatalf("session did not reconnect: %#v", status)
	}
	editor.ProcessCollaborationUpdates()
	assertEditorDirection(t, mapState, "2")
	if application.commands.HasUndoV("test") {
		t.Fatal("interrupted editor operation entered undo history")
	}

	instance = mapState.Tiles[0].Instances()[2]
	editor.InstanceReplace(instance, dmmprefab.New(dmmprefab.IdNone, instance.Prefab().Path(), dmvars.Set(instance.Prefab().Vars(), "dir", "8")))
	editor.CommitOperation("Fresh Real-Service Change")
	application.runScheduled(t)
	editor.ProcessCollaborationUpdates()
	assertEditorDirection(t, mapState, "8")
	if !application.commands.UndoAsyncV("test", nil) {
		t.Fatal("fresh acknowledged edit did not enter undo history")
	}
	application.runScheduled(t)
	editor.ProcessCollaborationUpdates()
	assertEditorDirection(t, mapState, "2")
}

func TestEditorAttachesRemoteExecutorAndAppliesSnapshot(t *testing.T) {
	dmmap.PrefabStorage.Free()
	t.Cleanup(dmmap.PrefabStorage.Free)

	environment := editorTestEnvironment()
	mapState := editorTestMap(environment)
	application := &editorTestApp{
		commands:    command.NewStorage(),
		environment: environment,
		paths:       dm.NewPathsFilterEmpty(),
	}
	application.commands.SetStack("test")
	editor := New(application, &editorTestAttachedMap{snapshot: dmmsnap.New(mapState)}, mapState)
	if editor.collaborationErr != nil {
		t.Fatalf("initialize collaboration: %v", editor.collaborationErr)
	}
	remote := model.CloneSnapshot(editor.authoritative)
	remoteDocumentID, err := model.NewDocumentID()
	if err != nil {
		t.Fatal(err)
	}
	remote.DocumentID = remoteDocumentID
	remote.Revision = 7
	remote.Tiles[0].State.Prefabs[2].Vars["dir"] = "4"
	document, err := engine.NewDocument(remote)
	if err != nil {
		t.Fatal(err)
	}
	remoteExecutor, err := executor.NewLocal(document, editor.actorID)
	if err != nil {
		t.Fatal(err)
	}

	if err := editor.AttachCollaborationExecutor(remoteExecutor); err != nil {
		t.Fatalf("attach remote executor: %v", err)
	}
	assertEditorDirection(t, mapState, "4")
	if editor.authoritative.DocumentID != remote.DocumentID || editor.authoritative.Revision != remote.Revision {
		t.Fatalf("authoritative identity = (%q, %d), want (%q, %d)", editor.authoritative.DocumentID, editor.authoritative.Revision, remote.DocumentID, remote.Revision)
	}
}

type editorTestApp struct {
	commands    *command.Storage
	environment *dmenv.Dme
	paths       *dm.PathsFilter
	runLater    chan func()
}

func (*editorTestApp) DoSelectPrefab(*dmmprefab.Prefab)          {}
func (*editorTestApp) DoEditInstance(*dmminstance.Instance)      {}
func (*editorTestApp) ShowLayout(string, bool)                   {}
func (*editorTestApp) SyncPrefabs()                              {}
func (*editorTestApp) SyncVarEditor()                            {}
func (*editorTestApp) SelectedPrefab() (*dmmprefab.Prefab, bool) { return nil, false }
func (*editorTestApp) Clipboard() *dmmclip.Clipboard             { return nil }
func (*editorTestApp) Prefs() prefs.Prefs                        { return prefs.Prefs{} }
func (app *editorTestApp) CommandStorage() *command.Storage      { return app.commands }
func (app *editorTestApp) PathsFilter() *dm.PathsFilter          { return app.paths }
func (app *editorTestApp) LoadedEnvironment() *dmenv.Dme         { return app.environment }
func (app *editorTestApp) RunLater(job func()) {
	app.runLater <- job
}

func (app *editorTestApp) runScheduled(t *testing.T) {
	t.Helper()
	select {
	case job := <-app.runLater:
		job()
	case <-time.After(time.Second):
		t.Fatal("no UI-thread job was scheduled")
	}
}

func (app *editorTestApp) discardScheduled(t *testing.T) {
	t.Helper()
	select {
	case <-app.runLater:
	case <-time.After(time.Second):
		t.Fatal("no UI-thread job was scheduled")
	}
}

type editorNetworkTransport struct {
	sent chan protocol.ClientEnvelope
}

type blockingEditorSessionTransport struct {
	collabui.SessionTransport
	intercepted chan struct{}
	release     chan struct{}
	once        sync.Once
}

func (transport *blockingEditorSessionTransport) Send(ctx context.Context, envelope protocol.ClientEnvelope) error {
	if envelope.Type != protocol.ClientOperationSubmit {
		return transport.SessionTransport.Send(ctx, envelope)
	}
	blocked := false
	transport.once.Do(func() {
		blocked = true
		close(transport.intercepted)
	})
	if !blocked {
		return transport.SessionTransport.Send(ctx, envelope)
	}
	select {
	case <-transport.release:
		return errors.New("forced disconnect before editor operation send")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newEditorNetworkTransport() *editorNetworkTransport {
	return &editorNetworkTransport{sent: make(chan protocol.ClientEnvelope, 4)}
}

func (*editorNetworkTransport) Connect(context.Context, protocol.JoinRequest, func(protocol.ServerEnvelope)) error {
	return nil
}

func (transport *editorNetworkTransport) Send(_ context.Context, envelope protocol.ClientEnvelope) error {
	transport.sent <- envelope
	return nil
}

func (*editorNetworkTransport) Close(websocket.StatusCode, string) error { return nil }

func (transport *editorNetworkTransport) next(t *testing.T) protocol.ClientEnvelope {
	t.Helper()
	select {
	case envelope := <-transport.sent:
		return envelope
	case <-time.After(time.Second):
		t.Fatal("network transport received no message")
		return protocol.ClientEnvelope{}
	}
}

func mustEditorJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type deferredAsyncExecutor struct {
	local     *executor.Local
	mutex     sync.Mutex
	operation model.Operation
	complete  func(model.AcceptedOperation, error)
}

type projectionTestExecutor struct {
	*executor.Local
	updates chan client.Projection
}

func (execution *projectionTestExecutor) ProjectionUpdates() <-chan client.Projection {
	return execution.updates
}

func newDeferredAsyncExecutor(t *testing.T, snapshot model.Snapshot, actorID model.ActorID) *deferredAsyncExecutor {
	t.Helper()
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	local, err := executor.NewLocal(document, actorID)
	if err != nil {
		t.Fatal(err)
	}
	return &deferredAsyncExecutor{local: local}
}

func (execution *deferredAsyncExecutor) Execute(ctx context.Context, operation model.Operation) (model.AcceptedOperation, error) {
	return execution.local.Execute(ctx, operation)
}

func (execution *deferredAsyncExecutor) ExecuteAsync(_ context.Context, operation model.Operation, complete func(model.AcceptedOperation, error)) error {
	execution.mutex.Lock()
	defer execution.mutex.Unlock()
	execution.operation = model.CloneOperation(operation)
	execution.complete = complete
	return nil
}

func (execution *deferredAsyncExecutor) BuildInverse(ctx context.Context, operationID model.OperationID) (model.Operation, error) {
	return execution.local.BuildInverse(ctx, operationID)
}

func (execution *deferredAsyncExecutor) Snapshot(ctx context.Context) (model.Snapshot, error) {
	return execution.local.Snapshot(ctx)
}

func (execution *deferredAsyncExecutor) resolve(t *testing.T) {
	t.Helper()
	execution.mutex.Lock()
	operation, complete := execution.operation, execution.complete
	execution.mutex.Unlock()
	if complete == nil {
		t.Fatal("network operation was not submitted asynchronously")
	}
	accepted, err := execution.local.Execute(context.Background(), operation)
	completed := make(chan struct{})
	go func() {
		complete(accepted, err)
		close(completed)
	}()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("async completion did not return")
	}
}

type editorTestAttachedMap struct {
	snapshot *dmmsnap.DmmSnap
}

func (*editorTestAttachedMap) ActiveLevel() int                                  { return 1 }
func (*editorTestAttachedMap) SetActiveLevel(int)                                {}
func (attached *editorTestAttachedMap) Snapshot() *dmmsnap.DmmSnap               { return attached.snapshot }
func (*editorTestAttachedMap) Size() imgui.Vec2                                  { return imgui.Vec2{} }
func (*editorTestAttachedMap) Canvas() *canvas.Canvas                            { return &canvas.Canvas{} }
func (*editorTestAttachedMap) CanvasState() *canvas.State                        { return nil }
func (*editorTestAttachedMap) CanvasControl() *canvas.Control                    { return nil }
func (*editorTestAttachedMap) CanvasOverlay() *canvas.Overlay                    { return nil }
func (*editorTestAttachedMap) PushAreaHover(util.Bounds, util.Color, util.Color) {}
func (*editorTestAttachedMap) OnMapSizeChange()                                  {}

func editorTestEnvironment() *dmenv.Dme {
	objects := make(map[string]*dmenv.Object)
	for _, path := range []string{"/area/foo", "/turf/foo", "/obj/foo"} {
		variables := &dmvars.MutableVariables{}
		variables.Put("dir", "2")
		objects[path] = &dmenv.Object{Path: path, Vars: variables.ToImmutable()}
	}
	return &dmenv.Dme{Objects: objects}
}

func editorTestMap(environment *dmenv.Dme) *dmmap.Dmm {
	coord := util.Point{X: 1, Y: 1, Z: 1}
	prefabs := make([]*dmmprefab.Prefab, 0, 3)
	for _, path := range []string{"/area/foo", "/turf/foo", "/obj/foo"} {
		variables := dmvars.FromParent(environment.Objects[path].Vars)
		if path == "/obj/foo" {
			variables = dmvars.Set(variables, "dir", "2")
		}
		prefabs = append(prefabs, dmmprefab.New(dmmprefab.IdNone, path, variables))
	}
	tile := &dmmap.Tile{Coord: coord}
	for _, prefab := range prefabs {
		tile.InstancesAdd(prefab)
	}
	return &dmmap.Dmm{MaxX: 1, MaxY: 1, MaxZ: 1, Tiles: []*dmmap.Tile{tile}}
}

func assertEditorDirection(t *testing.T, mapState *dmmap.Dmm, want string) {
	t.Helper()
	got := mapState.Tiles[0].Instances()[2].Prefab().Vars().ValueV("dir", "")
	if got != want {
		t.Fatalf("object direction = %q, want %q", got, want)
	}
}
