package http

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func proxyRequest(w http.ResponseWriter, r *http.Request, backend string) {
	target, err := url.Parse("http://" + backend)
	if err != nil {
		http.Error(w, "invalid backend", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
}
