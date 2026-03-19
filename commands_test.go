package e2b

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mdsq1/e2b/internal/connectrpc"
)

func TestCommandsRunUsesProcessProcessService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process.Process/Start" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		start, _ := json.Marshal(map[string]any{
			"event": map[string]any{
				"start": map[string]any{"pid": 42},
			},
		})
		_, _ = w.Write(connectrpc.EncodeEnvelope(start))

		end, _ := json.Marshal(map[string]any{
			"event": map[string]any{
				"end": map[string]any{"exitCode": 0},
			},
		})
		_, _ = w.Write(connectrpc.EncodeEnvelope(end))
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
	result, err := sbx.Commands.Run(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", result.ExitCode)
	}
}
