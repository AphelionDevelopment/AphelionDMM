package mapadapter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"sdmm/internal/dmapi/dmenv"
)

func EnvironmentHash(environment *dmenv.Dme) (string, error) {
	if environment == nil {
		return "", fmt.Errorf("hash environment: environment is nil")
	}
	paths := make([]string, 0, len(environment.Objects))
	for path := range environment.Objects {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var encoded bytes.Buffer
	writeAdapterString(&encoded, "apheliondmm.environment.v1")
	writeAdapterUint64(&encoded, uint64(len(paths)))
	for _, path := range paths {
		object := environment.Objects[path]
		if object == nil || object.Vars == nil {
			return "", fmt.Errorf("hash environment: object %q has nil variables", path)
		}
		writeAdapterString(&encoded, path)
		names := append([]string(nil), object.Vars.Iterate()...)
		sort.Strings(names)
		writeAdapterUint64(&encoded, uint64(len(names)))
		for _, name := range names {
			value, exists := object.Vars.Value(name)
			if !exists {
				return "", fmt.Errorf("hash environment: variable %q on %q has no value", name, path)
			}
			writeAdapterString(&encoded, name)
			writeAdapterString(&encoded, value)
		}
	}
	digest := sha256.Sum256(encoded.Bytes())
	return hex.EncodeToString(digest[:]), nil
}
