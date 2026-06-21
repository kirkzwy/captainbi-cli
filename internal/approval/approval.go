package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/core"
)

const validity = 15 * time.Minute

type Payload struct {
	Method          string            `json:"method"`
	Path            string            `json:"path"`
	Query           map[string]string `json:"query,omitempty"`
	Body            any               `json:"body,omitempty"`
	ContentType     string            `json:"content_type,omitempty"`
	ChannelID       string            `json:"channel_id,omitempty"`
	RiskLevel       string            `json:"risk_level"`
	RegistryVersion string            `json:"registry_version"`
}

type Record struct {
	RequestHash string    `json:"request_hash"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func Issue(payload Payload) (Record, error) {
	hash, err := Hash(payload)
	if err != nil {
		return Record{}, err
	}
	record := Record{RequestHash: hash, ExpiresAt: time.Now().Add(validity)}
	dir, err := approvalDir()
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Record{}, err
	}
	b, err := json.Marshal(record)
	if err != nil {
		return Record{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, hash+".json"), b, 0o600); err != nil {
		return Record{}, err
	}
	return record, nil
}

func Verify(payload Payload, provided string) error {
	expected, err := Hash(payload)
	if err != nil {
		return err
	}
	if provided == "" {
		return errors.New("confirm-request hash is required")
	}
	if provided != expected {
		return errors.New("confirm-request hash does not match the current request")
	}
	dir, err := approvalDir()
	if err != nil {
		return err
	}
	// #nosec G304 -- expected is a locally computed SHA-256 hex digest, never user-supplied path data.
	b, err := os.ReadFile(filepath.Join(dir, expected+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("confirm-request preview was not found or was already used")
		}
		return err
	}
	var record Record
	if err := json.Unmarshal(b, &record); err != nil {
		return err
	}
	if record.RequestHash != expected {
		return errors.New("confirm-request record is invalid")
	}
	if time.Now().After(record.ExpiresAt) {
		_ = os.Remove(filepath.Join(dir, expected+".json"))
		return errors.New("confirm-request preview has expired")
	}
	return nil
}

func Consume(requestHash string) error {
	if requestHash == "" {
		return nil
	}
	dir, err := approvalDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, requestHash+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func Hash(payload Payload) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("canonicalize approval request: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func approvalDir() (string, error) {
	dir, err := core.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "approvals"), nil
}
