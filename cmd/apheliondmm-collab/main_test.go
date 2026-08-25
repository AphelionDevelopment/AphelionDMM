package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
	"sdmm/internal/aphelion/collab/server"
	sqlitestore "sdmm/internal/aphelion/collab/store/sqlite"
)

func TestLoadSnapshotAndProtectedTokenFile(t *testing.T) {
	t.Parallel()

	snapshot, err := loadSnapshot(filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.MaxX != 3 || snapshot.MaxY != 1 || snapshot.MaxZ != 1 {
		t.Fatalf("snapshot dimensions = %dx%dx%d", snapshot.MaxX, snapshot.MaxY, snapshot.MaxZ)
	}
	path := filepath.Join(t.TempDir(), "launch.token")
	if err := writeLaunchToken(path, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := writeLaunchToken(path, "replacement"); err == nil {
		t.Fatal("writeLaunchToken replaced an existing token file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret" {
		t.Fatalf("token contents = %q", data)
	}
}

func TestRunExercisesServiceEntryPoint(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdoutReader, stdoutWriter := io.Pipe()
	defer func() { _ = stdoutReader.Close() }()
	var stderr bytes.Buffer
	tokenPath := filepath.Join(t.TempDir(), "launch.token")
	exitCodes := make(chan int, 1)
	go func() {
		exitCodes <- run(ctx, []string{
			"-snapshot-config", filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json"),
			"-listen", "127.0.0.1:0",
			"-launch-token-file", tokenPath,
		}, stdoutWriter, &stderr)
		_ = stdoutWriter.Close()
	}()

	scanner := bufio.NewScanner(stdoutReader)
	if !scanner.Scan() {
		t.Fatalf("entry point produced no address: %s", stderr.String())
	}
	baseURL := strings.TrimSuffix(strings.TrimPrefix(scanner.Text(), "base_url=\""), "\"")
	client := &http.Client{Timeout: time.Second}
	for _, path := range []string{"/v1/health/live", "/v1/version"} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			t.Fatalf("GET %s status = %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadSnapshot(filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(token))
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want 201", response.StatusCode)
	}

	cancel()
	select {
	case exitCode := <-exitCodes:
		if exitCode != 0 {
			t.Fatalf("run() exit code = %d, stderr = %s", exitCode, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("entry point did not shut down within two seconds")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("launch token file remains after shutdown: %v", err)
	}
}

func TestRunRecoversAcknowledgedOperationFromSQLite(t *testing.T) {
	snapshotPath := filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json")
	snapshot, err := loadSnapshot(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(t.TempDir(), "collaboration.db")

	first := startTestCommand(t, snapshotPath, databasePath)
	created, launchToken := createCommandSession(t, first.baseURL, first.tokenPath, snapshot)
	accepted := submitCommandOperation(t, first.baseURL, created, snapshot)
	waitForCommandSnapshot(t, databasePath, snapshot.DocumentID, accepted.Operation.Revision)
	first.stop(t)

	database, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for name, credential := range map[string]string{"launch": launchToken, "owner": created.OwnerToken} {
		if bytes.Contains(database, []byte(credential)) {
			t.Fatalf("SQLite database contains %s credential", name)
		}
	}

	second := startTestCommand(t, snapshotPath, databasePath)
	recovered, _ := createCommandSession(t, second.baseURL, second.tokenPath, snapshot)
	second.stop(t)
	if recovered.Revision != accepted.Operation.Revision {
		t.Fatalf("recovered revision = %d, want %d", recovered.Revision, accepted.Operation.Revision)
	}
	if recovered.MapHash != accepted.MapHash {
		t.Fatalf("recovered map hash = %q, want %q", recovered.MapHash, accepted.MapHash)
	}
}

func TestProcessEntryPointRecoversAcknowledgedOperationAfterForcedTermination(t *testing.T) {
	snapshotPath := filepath.Join("..", "..", "testdata", "collaboration", "snapshot.json")
	snapshot, err := loadSnapshot(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(t.TempDir(), "forced-termination.db")

	first := startCommandProcess(t, snapshotPath, databasePath)
	created, _ := createCommandSession(t, first.baseURL, first.tokenPath, snapshot)
	accepted := submitCommandOperation(t, first.baseURL, created, snapshot)
	first.terminate(t)

	second := startCommandProcess(t, snapshotPath, databasePath)
	recovered, _ := createCommandSession(t, second.baseURL, second.tokenPath, snapshot)
	second.terminate(t)
	if recovered.Revision != accepted.Operation.Revision || recovered.MapHash != accepted.MapHash {
		t.Fatalf("recovered state = revision %d hash %q, want revision %d hash %q", recovered.Revision, recovered.MapHash, accepted.Operation.Revision, accepted.MapHash)
	}
}

func TestCollaborationCommandProcess(t *testing.T) {
	if os.Getenv("APHELIONDMM_COMMAND_PROCESS") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		os.Exit(2)
	}
	os.Exit(run(context.Background(), os.Args[separator+1:], os.Stdout, os.Stderr))
}

type commandProcess struct {
	baseURL   string
	tokenPath string
	command   *exec.Cmd
	stderr    *bytes.Buffer
}

func startCommandProcess(t *testing.T, snapshotPath, databasePath string) commandProcess {
	t.Helper()
	tokenPath := filepath.Join(t.TempDir(), "launch.token")
	command := exec.Command(os.Args[0],
		"-test.run=^TestCollaborationCommandProcess$", "--",
		"-snapshot-config", snapshotPath,
		"-listen", "127.0.0.1:0",
		"-launch-token-file", tokenPath,
		"-database", databasePath,
	)
	command.Env = append(os.Environ(), "APHELIONDMM_COMMAND_PROCESS=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		_ = command.Wait()
		t.Fatalf("command process produced no address: %s", stderr.String())
	}
	process := commandProcess{
		baseURL:   strings.TrimSuffix(strings.TrimPrefix(scanner.Text(), "base_url=\""), "\""),
		tokenPath: tokenPath,
		command:   command,
		stderr:    stderr,
	}
	t.Cleanup(func() { process.terminate(t) })
	return process
}

func (process commandProcess) terminate(t *testing.T) {
	t.Helper()
	if process.command.ProcessState != nil {
		return
	}
	if err := process.command.Process.Kill(); err != nil {
		t.Fatalf("terminate command process: %v", err)
	}
	if err := process.command.Wait(); err == nil {
		t.Fatal("forcibly terminated command exited successfully")
	}
}

type testCommand struct {
	baseURL   string
	tokenPath string
	cancel    context.CancelFunc
	exitCode  <-chan int
	stderr    *bytes.Buffer
}

func startTestCommand(t *testing.T, snapshotPath, databasePath string) testCommand {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stdoutReader, stdoutWriter := io.Pipe()
	t.Cleanup(func() { _ = stdoutReader.Close() })
	stderr := &bytes.Buffer{}
	tokenPath := filepath.Join(t.TempDir(), "launch.token")
	exitCodes := make(chan int, 1)
	go func() {
		exitCodes <- run(ctx, []string{
			"-snapshot-config", snapshotPath,
			"-listen", "127.0.0.1:0",
			"-launch-token-file", tokenPath,
			"-database", databasePath,
			"-snapshot-operation-threshold", "1",
			"-snapshot-interval", "1h",
		}, stdoutWriter, stderr)
		close(exitCodes)
		_ = stdoutWriter.Close()
	}()
	scanner := bufio.NewScanner(stdoutReader)
	if !scanner.Scan() {
		cancel()
		t.Fatalf("entry point produced no address: %s", stderr.String())
	}
	command := testCommand{
		baseURL:   strings.TrimSuffix(strings.TrimPrefix(scanner.Text(), "base_url=\""), "\""),
		tokenPath: tokenPath,
		cancel:    cancel,
		exitCode:  exitCodes,
		stderr:    stderr,
	}
	t.Cleanup(func() { command.stop(t) })
	return command
}

func waitForCommandSnapshot(t *testing.T, databasePath string, documentID model.DocumentID, revision model.Revision) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		value, err := sqlitestore.Open(databasePath)
		if err == nil {
			snapshot, _, loadErr := value.Load(context.Background(), documentID)
			closeErr := value.Close()
			if loadErr == nil && closeErr == nil && snapshot.Revision == revision {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("durable snapshot did not reach revision %d", revision)
}

func (command testCommand) stop(t *testing.T) {
	t.Helper()
	command.cancel()
	select {
	case exitCode, open := <-command.exitCode:
		if !open {
			return
		}
		if exitCode != 0 {
			t.Fatalf("run() exit code = %d, stderr = %s", exitCode, command.stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("entry point did not shut down within two seconds")
	}
	if _, err := os.Stat(command.tokenPath); !os.IsNotExist(err) {
		t.Fatalf("launch token file remains after shutdown: %v", err)
	}
}

func createCommandSession(t *testing.T, baseURL, tokenPath string, snapshot model.Snapshot) (server.CreateSessionResponse, string) {
	t.Helper()
	launchTokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	launchToken := string(launchTokenBytes)
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+launchToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want 201", response.StatusCode)
	}
	var created server.CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return created, launchToken
}

func submitCommandOperation(t *testing.T, baseURL string, created server.CreateSessionResponse, snapshot model.Snapshot) protocol.OperationAcceptedPayload {
	t.Helper()
	connection, response, err := websocket.Dial(context.Background(), strings.Replace(baseURL, "http://", "ws://", 1)+"/v1/collaboration", &websocket.DialOptions{
		HTTPHeader:   http.Header{"Authorization": []string{"Bearer " + created.OwnerToken}, "Origin": []string{"http://127.0.0.1"}},
		Subprotocols: []string{server.WebSocketSubprotocol},
	})
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatal(err)
	}
	defer func() { _ = connection.CloseNow() }()
	writeCommandEnvelope(t, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "join", SessionID: created.SessionID, Type: protocol.ClientJoin}, protocol.JoinPayload{JoinToken: created.OwnerToken})
	joined := readCommandEnvelope(t, connection, protocol.ServerJoined).Payload.(*protocol.JoinedPayload)
	operationID, err := model.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	stableID, err := model.NewStableID()
	if err != nil {
		t.Fatal(err)
	}
	operation := model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		ActorID:         joined.ActorID,
		OperationID:     operationID,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     created.MapHash,
		Kind:            model.OperationKindTileChange,
		Changes: []model.TileChange{{
			Coord: model.Coord{X: 1, Y: 1, Z: 1},
			After: model.TileState{Prefabs: []model.PrefabState{{StableID: stableID, Path: "/turf/open/floor", Vars: map[string]string{}}}},
		}},
	}
	writeCommandEnvelope(t, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "operation", SessionID: created.SessionID, Type: protocol.ClientOperationSubmit}, protocol.OperationSubmitPayload{Operation: operation})
	accepted := readCommandEnvelope(t, connection, protocol.ServerOperationAccepted).Payload.(*protocol.OperationAcceptedPayload)
	writeCommandEnvelope(t, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "acknowledged", SessionID: created.SessionID, Type: protocol.ClientAcknowledgedRevision}, protocol.AcknowledgedRevisionPayload{Revision: accepted.Operation.Revision})
	return *accepted
}

func writeCommandEnvelope(t *testing.T, connection *websocket.Conn, envelope protocol.ClientEnvelope, payload any) {
	t.Helper()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Payload = encodedPayload
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := connection.Write(ctx, websocket.MessageText, encoded); err != nil {
		t.Fatal(err)
	}
}

func readCommandEnvelope(t *testing.T, connection *websocket.Conn, wanted protocol.ServerType) protocol.DecodedServer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for {
		_, data, err := connection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := protocol.DecodeServer(data)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Envelope.Type == wanted {
			return decoded
		}
	}
}
