package http

import (
	"io"
	"net/http"
)

type Proxy struct {
	transport RoundTripper
}

func NewProxy(transport RoundTripper) *Proxy {
	return &Proxy{
		transport: transport,
	}
}

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
