package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestClientFixturesDecode(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"client_join.json",
		"client_operation_submit.json",
		"client_inverse_request.json",
		"client_presence_update.json",
		"client_acknowledged_revision.json",
		"client_ping.json",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data := readFixture(t, name)
			if _, err := DecodeClient(data); err != nil {
				t.Fatalf("DecodeClient() error = %v", err)
			}
		})
	}
}

func TestServerFixturesDecode(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"server_joined.json",
		"server_operation_accepted.json",
		"server_operation_rejected.json",
		"server_replay_complete.json",
		"server_presence_snapshot.json",
		"server_presence_update.json",
		"server_session_notice.json",
		"server_pong.json",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data := readFixture(t, name)
			if _, err := DecodeServer(data); err != nil {
				t.Fatalf("DecodeServer() error = %v", err)
			}
		})
	}
}

func TestDecodeClientRejectsInvalidEnvelope(t *testing.T) {
	t.Parallel()

	valid := readFixture(t, "client_ping.json")
	tests := map[string][]byte{
		"unknown envelope field": []byte(`{"protocol_version":1,"message_id":"m","session_id":"s","type":"ping","payload":{"nonce":"n"},"extra":true}`),
		"unknown message type":   []byte(`{"protocol_version":1,"message_id":"m","session_id":"s","type":"execute","payload":{}}`),
		"missing version":        []byte(`{"message_id":"m","session_id":"s","type":"ping","payload":{"nonce":"n"}}`),
		"oversized identifier":   []byte(strings.Replace(string(valid), `"message_id":"message-1"`, `"message_id":"`+strings.Repeat("x", MaxIdentifierBytes+1)+`"`, 1)),
		"trailing document":      append(append([]byte(nil), valid...), []byte(` {}`)...),
	}
	for name, data := range tests {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeClient(data); err == nil {
				t.Fatal("DecodeClient() error = nil")
			}
		})
	}
}

func TestDecodeClientRejectsInvalidOperation(t *testing.T) {
	t.Parallel()

	data := readFixture(t, "client_operation_submit.json")
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	payload := envelope["payload"].(map[string]any)
	operation := payload["operation"].(map[string]any)
	operation["environment_hash"] = "not-a-sha256"
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeClient(data); err == nil {
		t.Fatal("DecodeClient() error = nil")
	}
}

func TestContractsDeclareEveryFixtureMessage(t *testing.T) {
	t.Parallel()

	openAPI := readContract(t, "openapi.yaml")
	for _, route := range []string{
		"/v1/sessions:",
		"/v1/sessions/{session_id}:",
		"/v1/sessions/{session_id}/join-tokens:",
		"/v1/sessions/{session_id}/snapshot:",
		"/v1/sessions/{session_id}/exports:",
		"/v1/health/live:",
		"/v1/health/ready:",
		"/v1/version:",
	} {
		if !strings.Contains(openAPI, route) {
			t.Errorf("openapi.yaml missing %s", route)
		}
	}

	asyncAPI := readContract(t, "asyncapi.yaml")
	for _, messageType := range append(AllClientTypes(), AllServerTypes()...) {
		if !strings.Contains(asyncAPI, string(messageType)+":") {
			t.Errorf("asyncapi.yaml missing message %q", messageType)
		}
	}
}

func TestContractsAreValidPinnedYAMLDocuments(t *testing.T) {
	t.Parallel()

	for name, versionKey := range map[string]string{
		"openapi.yaml":  "openapi",
		"asyncapi.yaml": "asyncapi",
	} {
		name, versionKey := name, versionKey
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var document map[string]any
			if err := yaml.Unmarshal([]byte(readContract(t, name)), &document); err != nil {
				t.Fatalf("pinned yaml.v3 validator rejected contract: %v", err)
			}
			if document[versionKey] == nil || document["info"] == nil || document["components"] == nil {
				t.Fatalf("contract lacks required %s, info, or components roots", versionKey)
			}
		})
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "v1", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readContract(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "api", "collaboration", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
