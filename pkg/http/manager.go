package http

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
)

type Manager struct {
	mu       sync.RWMutex
	services map[string]*Service
	servers  map[string]*http.Server

	proxy *Proxy

	listeners map[string]net.Listener
}

type Service struct {
	Name     string
	Listen   string
	Backends []string

	next int
	mu   sync.Mutex
}

func NewManager() *Manager {
	transport := NewTransport()
	proxy := NewProxy(transport)

	return &Manager{
		services:  make(map[string]*Service),
		servers:   make(map[string]*http.Server),
		listeners: make(map[string]net.Listener),
		proxy:     proxy,
	}
}

func (m *Manager) AddService(service *Service) error {
	if service == nil {
		return fmt.Errorf("service is nil")
	}

	if service.Name == "" {
		return fmt.Errorf("service name is required")
	}

	if service.Listen == "" {
		return fmt.Errorf("listen address is required")
	}

	if len(service.Backends) == 0 {
		return fmt.Errorf("service %q has no backends", service.Name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.services[service.Name]; exists {
		return fmt.Errorf("service %q already exists", service.Name)
	}

	m.services[service.Name] = service

	return nil
}

func (m *Manager) StartService(name string) error {
	m.mu.RLock()
	service, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %q not found", name)
	}

	// HTTP request handling is implemented in handler.go.
	handler := newHandler(service, m.proxy)

	server := &http.Server{
		Addr:    service.Listen,
		Handler: handler,
	}

	// Create the listener before starting the goroutine.
	ln, err := net.Listen("tcp", service.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", service.Listen, err)
	}

	m.mu.Lock()
	m.servers[name] = server
	m.listeners[name] = ln
	m.mu.Unlock()

	go func() {
		log.Printf(
			"HTTP service %q listening on %s",
			service.Name,
			ln.Addr().String(),
		)

		if err := server.Serve(ln); err != nil &&
			err != http.ErrServerClosed {
			log.Printf(
				"HTTP service %q stopped: %v",
				service.Name,
				err,
			)
		}
	}()

	return nil
}

func (m *Manager) StopService(name string) error {
	m.mu.Lock()

	server, exists := m.servers[name]
	if exists {
		delete(m.servers, name)
		delete(m.listeners, name)
	}

	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("service %q is not running", name)
	}

	if err := server.Close(); err != nil {
		return fmt.Errorf("stop service %q: %w", name, err)
	}

	return nil
}

func (m *Manager) RemoveService(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.services[name]; !exists {
		return fmt.Errorf("service %q not found", name)
	}

	if _, running := m.servers[name]; running {
		return fmt.Errorf("service %q is still running", name)
	}

	delete(m.services, name)

	return nil
}

func (m *Manager) ServiceAddr(name string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ln, exists := m.listeners[name]
	if !exists {
		return "", fmt.Errorf("service %q is not running", name)
	}

	return ln.Addr().String(), nil
}
