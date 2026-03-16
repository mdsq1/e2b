package e2b

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamTimestampUnmarshalJSON(t *testing.T) {
	isoText := "2026-03-16T11:01:25.534525+00:00"
	isoTime, err := time.Parse(time.RFC3339Nano, isoText)
	if err != nil {
		t.Fatalf("failed to parse test timestamp: %v", err)
	}

	tests := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "unix_number", raw: `1742122885534`, want: 1742122885534},
		{name: "unix_string", raw: `"1742122885534"`, want: 1742122885534},
		{name: "rfc3339_string", raw: `"` + isoText + `"`, want: isoTime.UnixMilli()},
		{name: "empty_string", raw: `""`, want: 0},
		{name: "null", raw: `null`, want: 0},
		{name: "invalid_string", raw: `"not-a-time"`, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got streamTimestamp
			if err := json.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if int64(got) != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestParseResult_UsesTextResultBeforeText(t *testing.T) {
	primary := "main result"
	fallback := "secondary text"

	result := (&CodeInterpreter{}).parseResult(&streamEvent{
		IsMainResult: true,
		Text:         fallback,
		ResultText:   &primary,
	})

	if result.Text == nil || *result.Text != primary {
		t.Fatalf("expected text_result to win, got %#v", result.Text)
	}
	if !result.IsMainResult {
		t.Fatal("expected IsMainResult to be preserved")
	}
}

func TestParseResult_FallsBackToTextField(t *testing.T) {
	result := (&CodeInterpreter{}).parseResult(&streamEvent{
		Text: "plain text result",
	})

	if result.Text == nil || *result.Text != "plain text result" {
		t.Fatalf("expected text fallback, got %#v", result.Text)
	}
}

func TestRunCode_ParsesMixedStreamEventShapes(t *testing.T) {
	isoText := "2026-03-16T11:01:25.534525+00:00"
	isoTime, err := time.Parse(time.RFC3339Nano, isoText)
	if err != nil {
		t.Fatalf("failed to parse test timestamp: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/execute" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"type\":\"number_of_executions\",\"execution_count\":7}\n")
		fmt.Fprintf(w, "data: {\"type\":\"stdout\",\"text\":\"Hello World\\n\",\"timestamp\":%q}\n", isoText)
		fmt.Fprintf(w, "{\"type\":\"stderr\",\"text\":\"warn\\n\",\"timestamp\":\"not-a-time\"}\n")
		fmt.Fprintf(w, "{\"type\":\"result\",\"is_main_result\":true,\"text_result\":\"main result\"}\n")
		fmt.Fprintf(w, "{\"type\":\"result\",\"text\":\"secondary result\"}\n")
		fmt.Fprintf(w, "{\"type\":\"error\",\"name\":\"ValueError\",\"value\":\"bad value\",\"traceback\":\"traceback text\"}\n")
		fmt.Fprintf(w, "data: [DONE]\n")
	}))
	defer server.Close()

	ci := &CodeInterpreter{
		Sandbox: &Sandbox{
			ID:     "sandbox-test",
			client: &Client{config: ConnectionConfig{APIKey: "test-key"}},
		},
		jupyterURL:  server.URL,
		jupyterHTTP: server.Client(),
	}

	var stdoutMessages []OutputMessage
	var stderrMessages []OutputMessage
	var results []Result
	var execErrors []ExecutionError

	exec, err := ci.RunCode(
		context.Background(),
		`print("Hello World")`,
		WithOnCodeStdout(func(msg OutputMessage) {
			stdoutMessages = append(stdoutMessages, msg)
		}),
		WithOnCodeStderr(func(msg OutputMessage) {
			stderrMessages = append(stderrMessages, msg)
		}),
		WithOnResult(func(result Result) {
			results = append(results, result)
		}),
		WithOnError(func(execErr ExecutionError) {
			execErrors = append(execErrors, execErr)
		}),
	)
	if err != nil {
		t.Fatalf("RunCode returned error: %v", err)
	}

	if got := strings.Join(exec.Logs.Stdout, ""); got != "Hello World\n" {
		t.Fatalf("expected stdout to contain Hello World, got %q", got)
	}
	if got := strings.Join(exec.Logs.Stderr, ""); got != "warn\n" {
		t.Fatalf("expected stderr to contain warn, got %q", got)
	}
	if exec.ExecutionCount == nil || *exec.ExecutionCount != 7 {
		t.Fatalf("expected execution count 7, got %#v", exec.ExecutionCount)
	}
	if exec.Text() != "main result" {
		t.Fatalf("expected main result text, got %q", exec.Text())
	}
	if len(exec.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(exec.Results))
	}
	if exec.Results[1].Text == nil || *exec.Results[1].Text != "secondary result" {
		t.Fatalf("expected secondary result text, got %#v", exec.Results[1].Text)
	}
	if exec.Error == nil || exec.Error.Name != "ValueError" || exec.Error.Value != "bad value" {
		t.Fatalf("expected execution error to be parsed, got %#v", exec.Error)
	}
	if len(stdoutMessages) != 1 {
		t.Fatalf("expected 1 stdout callback, got %d", len(stdoutMessages))
	}
	if stdoutMessages[0].Timestamp != isoTime.UnixMilli() {
		t.Fatalf("expected stdout callback timestamp %d, got %d", isoTime.UnixMilli(), stdoutMessages[0].Timestamp)
	}
	if len(stderrMessages) != 1 {
		t.Fatalf("expected 1 stderr callback, got %d", len(stderrMessages))
	}
	if stderrMessages[0].Timestamp != 0 {
		t.Fatalf("expected invalid stderr timestamp to downgrade to 0, got %d", stderrMessages[0].Timestamp)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 result callbacks, got %d", len(results))
	}
	if len(execErrors) != 1 || execErrors[0].Name != "ValueError" {
		t.Fatalf("expected 1 error callback, got %#v", execErrors)
	}
}
