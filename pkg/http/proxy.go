package http

import (
	"io"
	"net/http"
)

// Proxy is a simple HTTP reverse proxy that forwards requests to a specified backend.
type Proxy struct {
	transport RoundTripper
}

// NewProxy creates a new Proxy with the given RoundTripper.
func NewProxy(transport RoundTripper) *Proxy {
	return &Proxy{
		transport: transport,
	}
}

// ServeHTTP handles incoming HTTP requests and forwards them to the specified backend.
func (p *Proxy) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
	backend string,
) {
	resp, err := p.transport.RoundTrip(r, backend)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	defer resp.Body.Close()

	// Copy response headers.
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status.
	w.WriteHeader(resp.StatusCode)

	// Copy body.
	_, _ = io.Copy(w, resp.Body)
}
