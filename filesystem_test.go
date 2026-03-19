package e2b

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFilesystemListUsesFilesystemFilesystemService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/filesystem.Filesystem/ListDir" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entries": []any{}})
	}))
	defer server.Close()

	client, err := NewClient(
		WithAPIKey("test-key"),
		WithSandboxURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	sbx := client.newSandbox("sandbox-1", "test.e2b.local", "0.4.0", "token", "traffic")
	entries, err := sbx.Files.List(context.Background(), "/home/user/workspace")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty entries, got %d", len(entries))
	}
}
