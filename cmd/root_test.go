package cmd

import "testing"

func TestRootCommands(t *testing.T) {
	root := NewRootCmd()
	names := commandNames(root)
	want := map[string]bool{"api": true, "auth": true, "config": true, "doctor": true, "schema": true}
	for _, n := range names {
		delete(want, n)
	}
	for n := range want {
		t.Fatalf("missing root command %s; got %v", n, names)
	}
}
