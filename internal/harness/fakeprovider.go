// Package harness implements the shared real-Runtime Test Harness: it
// prepares a disposable project and isolated Runtime environment, invokes
// the production Installer, launches a real pinned Runtime against a
// loopback fake model provider, and asserts on the first outbound request.
// The OpenCode case reuses this package; Codex-specific launch and request
// decoding live in the conformance cases.
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
// requests and returns a valid canned streaming completion so the real
// Runtime exits successfully. It never uses credentials, model cost, or a
// real LLM.
type FakeProvider struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []Request
}

// NewFakeProvider starts a loopback fake provider on 127.0.0.1.
func NewFakeProvider() *FakeProvider {
	p := &FakeProvider{}
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

// ResponsesRequests returns the captured model requests, i.e. the requests
// that are not provider metadata probes.
func (p *FakeProvider) ResponsesRequests() []Request {
	var out []Request
	for _, r := range p.Requests() {
		if r.Method == http.MethodPost && r.Path == "/v1/responses" {
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
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, cannedCompletion)
	default:
		http.NotFound(w, r)
	}
}

// cannedCompletion is a valid Responses API streaming completion: a created
// response, one assistant output item, and a completed response. It mirrors
// the event shape the pinned Codex CLI's own test suite serves.
const cannedCompletion = `event: response.created
data: {"type":"response.created","response":{"id":"response_1"}}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","id":"response_1","content":[{"type":"output_text","text":"done"}]}}

event: response.completed
data: {"type":"response.completed","response":{"id":"response_1","usage":{"input_tokens":0,"input_tokens_details":null,"output_tokens":0,"output_tokens_details":null,"total_tokens":0}}}

`
