package registry

import (
	_ "embed"
	"encoding/json"
)

//go:embed captainbi_meta.json
var embedded []byte

func Load() (*Registry, error) {
	var r Registry
	if err := json.Unmarshal(embedded, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
