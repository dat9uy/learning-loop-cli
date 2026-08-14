package harness

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFakeProviderCapturesResponsesRequest(t *testing.T) {
	p := NewFakeProvider()
	defer p.Close()

	body := `{"model":"gpt-5","input":[]}`
	resp, err := http.Post(p.URL()+"/v1/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/responses: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if !strings.Contains(string(data), "response.completed") {
		t.Fatalf("canned completion missing response.completed: %q", data)
	}

	reqs := p.ResponsesRequests()
	if len(reqs) != 1 {
		t.Fatalf("captured %d responses requests, want 1", len(reqs))
	}
	if reqs[0].Method != http.MethodPost || reqs[0].Path != "/v1/responses" {
		t.Fatalf("request = %s %s, want POST /v1/responses", reqs[0].Method, reqs[0].Path)
	}
	if string(reqs[0].Body) != body {
		t.Fatalf("body = %q, want %q", reqs[0].Body, body)
	}
}

func TestFakeProviderServesModelsProbe(t *testing.T) {
	p := NewFakeProvider()
	defer p.Close()

	resp, err := http.Get(p.URL() + "/v1/models")
	if err != nil {
		t.Fatalf("GET /v1/models: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), `"models"`) {
		t.Fatalf("models response = %q, want models list", data)
	}
	if got := p.ResponsesRequests(); len(got) != 0 {
		t.Fatalf("models probe counted as a model request: %d", len(got))
	}
}

func TestFakeProviderUnknownPathIsNotFound(t *testing.T) {
	p := NewFakeProvider()
	defer p.Close()

	resp, err := http.Get(p.URL() + "/v1/other")
	if err != nil {
		t.Fatalf("GET /v1/other: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
