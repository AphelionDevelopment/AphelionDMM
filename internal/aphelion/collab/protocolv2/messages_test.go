package protocolv2

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplicationMessageRoundTripUsesTypedStrictPayload(t *testing.T) {
	encoded, err := EncodeApplication(ApplicationProfileUpdate, ProfileUpdate{DisplayName: "Test Mapper", Sequence: 4})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"profile_update","payload":{"display_name":"Test Mapper","sequence":4}}`, string(encoded))

	decoded, err := DecodeApplication(encoded)
	require.NoError(t, err)
	require.Equal(t, ApplicationProfileUpdate, decoded.Type)
	require.Equal(t, &ProfileUpdate{DisplayName: "Test Mapper", Sequence: 4}, decoded.Payload)
}

func TestApplicationMessageRejectsUnknownTrailingAndOversizedInput(t *testing.T) {
	tests := map[string][]byte{
		"unknown envelope field": []byte(`{"type":"profile_update","payload":{"display_name":"Mapper","sequence":1},"extra":true}`),
		"unknown payload field":  []byte(`{"type":"profile_update","payload":{"display_name":"Mapper","sequence":1,"extra":true}}`),
		"unknown type":           []byte(`{"type":"not_a_message","payload":{}}`),
		"trailing value":         []byte(`{"type":"profile_update","payload":{"display_name":"Mapper","sequence":1}} {}`),
		"oversized":              bytes.Repeat([]byte{'x'}, MaxApplicationBytes+1),
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeApplication(encoded)
			require.Error(t, err)
		})
	}
}

func TestEncodeApplicationRejectsMismatchedPayloadType(t *testing.T) {
	_, err := EncodeApplication(ApplicationProfileUpdate, SyncComplete{})
	require.Error(t, err)
	_, err = EncodeApplication(ApplicationType(strings.Repeat("x", 200)), struct{}{})
	require.Error(t, err)
}
