package connectrpc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncodeDecodeEnvelope(t *testing.T) {
	data := []byte(`{"hello":"world"}`)
	encoded := EncodeEnvelope(data)

	if len(encoded) != 5+len(data) {
		t.Fatalf("expected length %d, got %d", 5+len(data), len(encoded))
	}

	flags, dataLen := DecodeEnvelopeHeader(encoded[:5])
	if flags != 0 {
		t.Errorf("expected flags=0, got %d", flags)
	}
	if int(dataLen) != len(data) {
		t.Errorf("expected dataLen=%d, got %d", len(data), dataLen)
	}

	decoded := encoded[5:]
	if string(decoded) != string(data) {
		t.Errorf("decoded data mismatch: %q vs %q", string(decoded), string(data))
	}
}

func TestCallUnary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("expected Connect-Protocol-Version 1, got %s", r.Header.Get("Connect-Protocol-Version"))
		}
		if r.Header.Get("X-Custom") != "test" {
			t.Errorf("expected custom header X-Custom=test, got %s", r.Header.Get("X-Custom"))
		}

		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": req["input"] + "_ok"})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Headers:    map[string]string{"X-Custom": "test"},
	}

	req := map[string]string{"input": "hello"}
	var resp map[string]string
	err := client.CallUnary(context.Background(), "test.Service", "Method", req, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp["result"] != "hello_ok" {
		t.Errorf("expected result 'hello_ok', got %q", resp["result"])
	}
}

func TestCallUnaryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"code": "not_found", "message": "resource missing"})
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	err := client.CallUnary(context.Background(), "test.Service", "Method", struct{}{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if connErr.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", connErr.Code)
	}
}

func TestCallServerStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/connect+json" {
			t.Errorf("expected Content-Type application/connect+json, got %s", r.Header.Get("Content-Type"))
		}

		// Send two messages then end_stream trailer
		msg1, _ := json.Marshal(map[string]int{"value": 1})
		w.Write(EncodeEnvelope(msg1))

		msg2, _ := json.Marshal(map[string]int{"value": 2})
		w.Write(EncodeEnvelope(msg2))

		// end_stream trailer (flags=0x02)
		trailer, _ := json.Marshal(map[string]interface{}{})
		header := EncodeEnvelope(trailer)
		header[0] = 0x02 // set end_stream flag
		w.Write(header)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	stream, err := client.CallServerStream(context.Background(), "test.Service", "Stream", struct{}{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var msg1 map[string]int
	if err := stream.Next(&msg1); err != nil {
		t.Fatalf("failed to read msg1: %v", err)
	}
	if msg1["value"] != 1 {
		t.Errorf("expected value=1, got %d", msg1["value"])
	}

	var msg2 map[string]int
	if err := stream.Next(&msg2); err != nil {
		t.Fatalf("failed to read msg2: %v", err)
	}
	if msg2["value"] != 2 {
		t.Errorf("expected value=2, got %d", msg2["value"])
	}

	// Should get io.EOF on end_stream
	var msg3 map[string]int
	err = stream.Next(&msg3)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestCallServerStreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// end_stream trailer with error
		trailer, _ := json.Marshal(map[string]interface{}{
			"error": map[string]string{
				"code":    "not_found",
				"message": "thing not found",
			},
		})
		header := EncodeEnvelope(trailer)
		header[0] = 0x02
		w.Write(header)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	stream, err := client.CallServerStream(context.Background(), "test.Service", "Stream", struct{}{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var msg map[string]int
	err = stream.Next(&msg)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if connErr.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", connErr.Code)
	}
}
