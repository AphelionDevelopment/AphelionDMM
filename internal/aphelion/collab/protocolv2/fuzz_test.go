package protocolv2

import "testing"

func FuzzUnmarshalEnvelope(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, wireHeaderSize))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = UnmarshalEnvelope(data)
	})
}

func FuzzDecodeApplication(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte(`{"type":"presence_update","payload":{"sequence":1,"status":"active"}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeApplication(data)
	})
}
