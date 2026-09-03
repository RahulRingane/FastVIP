package http

import (
	"log"
	"net/http"
)

func newHandler(service *Service, proxy *Proxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend := service.nextBackend()

		if backend == "" {
			http.Error(w, "no backend available", http.StatusServiceUnavailable)
			return
		}

		log.Printf("http %s %s -> %s", r.Method, r.URL.Path, backend)

		proxy.ServeHTTP(w, r, backend)
	})
}
