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
	requestBodies := 0
	getBodySchemas := 0
	posts := 0
	for _, method := range r.AllMethods() {
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
	if requestBodies != 36 || getBodySchemas != 28 || posts != 8 {
		t.Fatalf("registry contract counts: requestBodies=%d getBodySchemas=%d posts=%d", requestBodies, getBodySchemas, posts)
	}
}
