package approval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/core"
)

func TestApprovalBindsAndConsumesRequest(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	payload := Payload{
		Method:          "POST",
		Path:            "/v1/open_finance/set_goods_cost",
		Body:            map[string]any{"data": []any{map[string]any{"sku": "TEST"}}},
		ContentType:     "multipart/form-data",
		ChannelID:       "channel",
		RiskLevel:       "write_dangerous",
		RegistryVersion: "0.3.0",
	}
	record, err := Issue(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, record.RequestHash); err != nil {
		t.Fatalf("verify issued approval: %v", err)
	}
	changed := payload
	changed.Body = map[string]any{"data": []any{map[string]any{"sku": "CHANGED"}}}
	if err := Verify(changed, record.RequestHash); err == nil {
		t.Fatal("changed request unexpectedly matched approval")
	}
	if err := Consume(record.RequestHash); err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, record.RequestHash); err == nil {
		t.Fatal("consumed approval was reusable")
	}
}

func TestExpiredApprovalIsRejected(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	payload := Payload{Method: "POST", Path: "/write", RiskLevel: "write_safe", RegistryVersion: "0.3.0"}
	hash, err := Hash(payload)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := approvalDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(Record{RequestHash: hash, ExpiresAt: time.Now().Add(-time.Minute)})
	if err := os.WriteFile(filepath.Join(dir, hash+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, hash); err == nil || err.Error() != "confirm-request preview has expired" {
		t.Fatalf("expected expired approval error, got %v", err)
	}
}
