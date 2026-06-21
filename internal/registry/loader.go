package registry

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirkzwy/captainbi-cli/internal/core"
	"github.com/kirkzwy/captainbi-cli/internal/lockfile"
)

const (
	EnvRegistryFile = "CAPTAINBI_REGISTRY_FILE"
	overrideName    = "registry.json"
)

//go:embed captainbi_meta.json
var embedded []byte

type LoadInfo struct {
	EmbeddedVersion  string `json:"embedded_version"`
	EffectiveVersion string `json:"effective_version"`
	OverridePath     string `json:"override_path,omitempty"`
	Overridden       bool   `json:"overridden"`
	Warning          string `json:"warning,omitempty"`
}

func Load() (*Registry, error) {
	r, _, err := LoadWithInfo()
	return r, err
}

func LoadWithInfo() (*Registry, LoadInfo, error) {
	baseline, err := parse(embedded)
	if err != nil {
		return nil, LoadInfo{}, fmt.Errorf("parse embedded registry: %w", err)
	}
	info := LoadInfo{EmbeddedVersion: baseline.Version, EffectiveVersion: baseline.Version}
	path, explicit, err := configuredOverridePath()
	if err != nil {
		info.Warning = err.Error()
		return baseline, info, nil
	}
	info.OverridePath = path
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return baseline, info, nil
	}
	if err != nil {
		if explicit {
			return nil, info, fmt.Errorf("read registry override: %w", err)
		}
		info.Warning = "registry override was ignored: " + err.Error()
		return baseline, info, nil
	}
	candidate, err := parse(b)
	if err == nil {
		err = ValidateOverride(candidate, baseline)
	}
	if err != nil {
		if explicit {
			return nil, info, fmt.Errorf("invalid registry override: %w", err)
		}
		info.Warning = "registry override was ignored: " + err.Error()
		return baseline, info, nil
	}
	info.EffectiveVersion = candidate.Version
	info.Overridden = true
	return candidate, info, nil
}

func OverridePath() (string, error) {
	path, _, err := configuredOverridePath()
	return path, err
}

func InstallOverride(ctx context.Context, data []byte) (LoadInfo, error) {
	baseline, err := parse(embedded)
	if err != nil {
		return LoadInfo{}, err
	}
	candidate, err := parse(data)
	if err != nil {
		return LoadInfo{}, fmt.Errorf("parse registry metadata: %w", err)
	}
	if err := ValidateOverride(candidate, baseline); err != nil {
		return LoadInfo{}, err
	}
	path, err := OverridePath()
	if err != nil {
		return LoadInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return LoadInfo{}, err
	}
	release, err := lockfile.Acquire(ctx, path+".lock")
	if err != nil {
		return LoadInfo{}, err
	}
	defer release()
	tmp, err := os.CreateTemp(filepath.Dir(path), ".registry-*.tmp")
	if err != nil {
		return LoadInfo{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return LoadInfo{}, err
	}
	formatted, err := json.MarshalIndent(candidate, "", "  ")
	if err == nil {
		_, err = tmp.Write(append(formatted, '\n'))
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return LoadInfo{}, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return LoadInfo{}, err
	}
	return LoadInfo{
		EmbeddedVersion:  baseline.Version,
		EffectiveVersion: candidate.Version,
		OverridePath:     path,
		Overridden:       true,
	}, nil
}

func RemoveOverride(ctx context.Context) (string, error) {
	path, err := OverridePath()
	if err != nil {
		return "", err
	}
	if os.Getenv(EnvRegistryFile) != "" {
		return "", fmt.Errorf("unset %s before resetting the managed registry override", EnvRegistryFile)
	}
	release, err := lockfile.Acquire(ctx, path+".lock")
	if err != nil {
		return "", err
	}
	defer release()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func ValidateOverride(candidate, baseline *Registry) error {
	if candidate == nil || baseline == nil {
		return errors.New("registry is nil")
	}
	if candidate.Version == "" || len(candidate.Services) == 0 {
		return errors.New("registry version and services are required")
	}
	if !strings.HasPrefix(candidate.Source, "https://doc.captainbi.com/") {
		return errors.New("registry source must be the official CaptainBI documentation domain")
	}
	candidateMethods, err := indexedMethods(candidate)
	if err != nil {
		return err
	}
	baselineMethods, err := indexedMethods(baseline)
	if err != nil {
		return err
	}
	for key, old := range baselineMethods {
		current, ok := candidateMethods[key]
		if !ok {
			return fmt.Errorf("registry removes existing command %s", key)
		}
		if current.HTTPMethod != old.HTTPMethod || current.FullPath != old.FullPath {
			return fmt.Errorf("registry changes method or path for %s", key)
		}
		if riskRank(current.RiskLevel) < riskRank(old.RiskLevel) {
			return fmt.Errorf("registry lowers risk for %s", key)
		}
		if old.RequiresOpenChannelID && !current.RequiresOpenChannelID {
			return fmt.Errorf("registry removes OpenChannelId requirement for %s", key)
		}
	}
	return nil
}

func configuredOverridePath() (string, bool, error) {
	if configured := strings.TrimSpace(os.Getenv(EnvRegistryFile)); configured != "" {
		path, err := filepath.Abs(configured)
		return path, true, err
	}
	dir, err := core.ConfigDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(dir, overrideName), false, nil
}

func parse(data []byte) (*Registry, error) {
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func indexedMethods(r *Registry) (map[string]Method, error) {
	methods := map[string]Method{}
	for _, service := range r.Services {
		if service.Domain == "" {
			return nil, errors.New("registry service domain is required")
		}
		for _, method := range service.Methods {
			key := service.Domain + "." + method.CommandName
			if method.CommandName == "" || method.FullPath == "" || method.HTTPMethod == "" {
				return nil, fmt.Errorf("registry command %s is incomplete", key)
			}
			if _, exists := methods[key]; exists {
				return nil, fmt.Errorf("registry command %s is duplicated", key)
			}
			if method.HTTPMethod != "GET" && method.HTTPMethod != "HEAD" && riskRank(method.RiskLevel) <= riskRank("read") {
				return nil, fmt.Errorf("write command %s must not use read risk", key)
			}
			methods[key] = method
		}
	}
	return methods, nil
}

func riskRank(risk string) int {
	switch risk {
	case "read":
		return 0
	case "write_safe":
		return 1
	case "write_dangerous":
		return 2
	case "sync_trigger":
		return 3
	default:
		return -1
	}
}
