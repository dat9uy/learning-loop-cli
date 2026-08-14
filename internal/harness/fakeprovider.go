// Package harness implements the shared real-Runtime Test Harness: it
// prepares a disposable project and isolated Runtime environment, invokes
// the production Installer, launches a real pinned Runtime against a
// loopback fake model provider, and asserts on the first outbound request.
// Each conformance case owns its native launch, test-only configuration,
// streaming response shape, and request decoding.
package harness

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Request is one captured outbound model request.
type Request struct {
	Method string
	Path   string
	Body   []byte
}

// FakeProvider is a loopback fake model provider. It captures outbound
// requests and returns the case's valid canned streaming completion so the
// real Runtime exits successfully. It never uses credentials, model cost,
// or a real LLM.
type FakeProvider struct {
	server     *httptest.Server
	mu         sync.Mutex
	requests   []Request
	modelPath  string
	completion string
}

// NewFakeProvider starts a loopback fake provider on 127.0.0.1 serving the
// given canned streaming completion at the given model request path.
func NewFakeProvider(modelPath, completion string) *FakeProvider {
	p := &FakeProvider{modelPath: modelPath, completion: completion}
	p.server = httptest.NewServer(http.HandlerFunc(p.handle))
	return p
}

// URL returns the provider's base URL, e.g. http://127.0.0.1:PORT.
func (p *FakeProvider) URL() string {
	return p.server.URL
}

// Close shuts the provider down.
func (p *FakeProvider) Close() {
	p.server.Close()
}

// Requests returns a copy of every captured request in arrival order.
func (p *FakeProvider) Requests() []Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Request(nil), p.requests...)
}

// ModelRequests returns the captured model requests, i.e. the requests that
// are not provider metadata probes.
func (p *FakeProvider) ModelRequests() []Request {
	var out []Request
	for _, r := range p.Requests() {
		if r.Method == http.MethodPost && r.Path == p.modelPath {
			out = append(out, r)
		}
	}
	return out
}

func (p *FakeProvider) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	p.mu.Lock()
	p.requests = append(p.requests, Request{Method: r.Method, Path: r.URL.Path, Body: body})
	p.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[]}`)
	case r.Method == http.MethodPost && r.URL.Path == p.modelPath:
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, p.completion)
	default:
		http.NotFound(w, r)
	}
}
