package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirkzwy/captainbi-cli/internal/core"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
)

func TestRegistryUpdateLoadAndResetCLI(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(registry.EnvRegistryFile, "")
	base, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	candidate := *base
	candidate.Version = "0.3.1-test"
	payload, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	oldSource, oldClient := registryMetadataSource, registryHTTPClient
	registryMetadataSource, registryHTTPClient = server.URL, server.Client()
	t.Cleanup(func() {
		registryMetadataSource, registryHTTPClient = oldSource, oldClient
	})

	globals = globalOptions{}
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"registry", "update", "--machine"})
	if err := root.Execute(); err != nil {
		t.Fatalf("registry update: %v", err)
	}
	var update map[string]any
	if err := json.Unmarshal(out.Bytes(), &update); err != nil {
		t.Fatal(err)
	}
	if update["ok"] != true || update["registry_version"] != candidate.Version {
		t.Fatalf("unexpected update output: %#v", update)
	}
	if data, ok := update["data"].(map[string]any); !ok || data["registry_version"] != candidate.Version {
		t.Fatalf("registry update missing compatible data envelope: %#v", update)
	}

	globals = globalOptions{}
	root = NewRootCmd()
	out.Reset()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doctor", "local", "--machine"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	var doctor map[string]any
	if err := json.Unmarshal(out.Bytes(), &doctor); err != nil {
		t.Fatal(err)
	}
	if doctor["registry_overridden"] != true || doctor["registry_version"] != candidate.Version {
		t.Fatalf("override not reported: %#v", doctor)
	}
	if data, ok := doctor["data"].(map[string]any); !ok || data["registry_version"] != candidate.Version {
		t.Fatalf("doctor missing compatible data envelope: %#v", doctor)
	}

	globals = globalOptions{}
	root = NewRootCmd()
	out.Reset()
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"registry", "reset", "--machine"})
	if err := root.Execute(); err != nil {
		t.Fatalf("registry reset: %v", err)
	}
	loaded, info, err := registry.LoadWithInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.Overridden || loaded.Version != base.Version {
		t.Fatalf("embedded registry not restored: version=%q info=%#v", loaded.Version, info)
	}
}

func TestCountOpenAPIMethodsAcceptsLowercaseKeys(t *testing.T) {
	count, err := countOpenAPIMethods([]byte(`{"paths":{"/a":{"get":{}},"/b":{"post":{},"parameters":[]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}
}
