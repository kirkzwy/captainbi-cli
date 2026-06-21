package registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirkzwy/captainbi-cli/internal/core"
)

func TestLoadRegistry(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Services) == 0 {
		t.Fatal("expected generated services")
	}
	if got := len(r.AllMethods()); got != 65 {
		t.Fatalf("expected 65 methods, got %d", got)
	}
	requestBodies := 0
	getBodySchemas := 0
	posts := 0
	rangeTypes := map[string]int{}
	for _, method := range r.AllMethods() {
		rangeTypes[method.Pagination.RangeType]++
		if method.ResponseSchema == nil {
			t.Fatalf("method %s missing responseSchema", method.Name)
		}
		if method.RequestBodySchema != nil {
			requestBodies++
		}
		if method.HTTPMethod == "GET" && method.RequestBodySchema != nil {
			getBodySchemas++
			if method.ContentType != "" {
				t.Fatalf("GET %s should encode documented body fields as query", method.Name)
			}
			for _, param := range method.Params {
				if param.Location == "form" {
					t.Fatalf("GET %s retained form parameter %s", method.Name, param.Name)
				}
			}
		}
		if method.HTTPMethod == "POST" {
			posts++
			if method.RequestBodySchema == nil || method.ContentType != "multipart/form-data" {
				t.Fatalf("POST %s missing multipart request schema", method.Name)
			}
		}
		flags := map[string]bool{}
		for _, param := range method.Params {
			if flags[param.Flag] {
				t.Fatalf("method %s has duplicate flag %s", method.Name, param.Flag)
			}
			flags[param.Flag] = true
		}
	}
	if rangeTypes["modified_time_window"] != 34 || rangeTypes["report_date"] != 16 || rangeTypes[""] != 15 {
		t.Fatalf("unexpected range strategy counts: %#v", rangeTypes)
	}
	if requestBodies != 36 || getBodySchemas != 28 || posts != 8 {
		t.Fatalf("registry contract counts: requestBodies=%d getBodySchemas=%d posts=%d", requestBodies, getBodySchemas, posts)
	}
	for _, ref := range []string{"goods.set-group", "goods.set-operate-user"} {
		method, ok := r.Find(ref)
		if !ok {
			t.Fatalf("missing method %s", ref)
		}
		found := false
		for _, param := range method.Params {
			if param.Name == "goods_id" {
				found = true
				if !strings.Contains(param.Description, "amazon_goods_id") {
					t.Fatalf("%s goods_id description is ambiguous: %q", ref, param.Description)
				}
			}
		}
		if !found {
			t.Fatalf("%s missing goods_id parameter", ref)
		}
	}
}

func TestRegistryOverrideInstallLoadAndReset(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(EnvRegistryFile, "")
	base, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	candidate := *base
	candidate.Version = "0.3.1-test"
	b, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	info, err := InstallOverride(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Overridden || info.EffectiveVersion != candidate.Version {
		t.Fatalf("unexpected install info: %#v", info)
	}
	loaded, loadedInfo, err := LoadWithInfo()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != candidate.Version || !loadedInfo.Overridden {
		t.Fatalf("override not loaded: version=%q info=%#v", loaded.Version, loadedInfo)
	}
	if _, err := RemoveOverride(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded, loadedInfo, err = LoadWithInfo()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != base.Version || loadedInfo.Overridden {
		t.Fatalf("embedded registry not restored: version=%q info=%#v", loaded.Version, loadedInfo)
	}
}

func TestRegistryOverrideRejectsRiskDowngrade(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	t.Setenv(EnvRegistryFile, "")
	base, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	candidate := *base
	candidate.Services = append([]Service(nil), base.Services...)
	changed := false
	for i := range candidate.Services {
		candidate.Services[i].Methods = append([]Method(nil), base.Services[i].Methods...)
		for j := range candidate.Services[i].Methods {
			if candidate.Services[i].Methods[j].RiskLevel != "read" {
				candidate.Services[i].Methods[j].RiskLevel = "read"
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}
	b, _ := json.Marshal(candidate)
	if _, err := InstallOverride(context.Background(), b); err == nil || !strings.Contains(err.Error(), "risk") {
		t.Fatalf("expected risk downgrade rejection, got %v", err)
	}
}

func TestInvalidManagedOverrideFallsBackButExplicitFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(core.EnvConfigDir, dir)
	t.Setenv(EnvRegistryFile, "")
	path := filepath.Join(dir, overrideName)
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, info, err := LoadWithInfo()
	if err != nil || info.Warning == "" || info.Overridden {
		t.Fatalf("expected managed fallback warning, info=%#v err=%v", info, err)
	}
	t.Setenv(EnvRegistryFile, path)
	if _, _, err := LoadWithInfo(); err == nil {
		t.Fatal("expected explicit invalid override to fail")
	}
}
