package registry

import "testing"

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
}
