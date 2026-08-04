//go:build !integration

package lvs

import (
	"fmt"
	"sync"
)

// fakeServiceKey is used internally by fakeHandle to index services.
type fakeServiceKey struct {
	address  string
	port     uint16
	protocol uint16
}

func makeFakeServiceKey(svc *Service) fakeServiceKey {
	return fakeServiceKey{
		address:  svc.Address.String(),
		port:     svc.Port,
		protocol: svc.Protocol,
	}
}

// fakeDestinationKey is used internally by fakeHandle to index destinations.
type fakeDestinationKey struct {
	address string
	port    uint16
}

func makeFakeDestinationKey(dst *Destination) fakeDestinationKey {
	return fakeDestinationKey{
		address: dst.Address.String(),
		port:    dst.Port,
	}
}

// fakeHandle provides an in-memory IPVS implementation for non-Linux systems.
// It simulates IPVS kernel behavior using maps, enabling development and testing on macOS.
type fakeHandle struct {
	services     map[fakeServiceKey]*Service
	destinations map[fakeServiceKey]map[fakeDestinationKey]*Destination
	mu           sync.Mutex
}

// NewIPVSHandle creates a fake in-memory IPVS handle for non-Linux systems.
func NewIPVSHandle(_ string) (IPVSHandle, error) {
	return &fakeHandle{
		services:     make(map[fakeServiceKey]*Service),
		destinations: make(map[fakeServiceKey]map[fakeDestinationKey]*Destination),
	}, nil
}

func (h *fakeHandle) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.services = nil
	h.destinations = nil
}

func (h *fakeHandle) NewService(svc *Service) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := makeFakeServiceKey(svc)
	if _, exists := h.services[key]; exists {
		return fmt.Errorf("service %s:%d already exists", svc.Address, svc.Port)
	}

	h.services[key] = cloneService(svc)
	h.destinations[key] = make(map[fakeDestinationKey]*Destination)
	return nil
}

func (h *fakeHandle) UpdateService(svc *Service) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := makeFakeServiceKey(svc)
	if _, exists := h.services[key]; !exists {
		return fmt.Errorf("service %s:%d not found", svc.Address, svc.Port)
	}

	h.services[key] = cloneService(svc)
	return nil
}

func (h *fakeHandle) DelService(svc *Service) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := makeFakeServiceKey(svc)
	if _, exists := h.services[key]; !exists {
		return fmt.Errorf("service %s:%d not found", svc.Address, svc.Port)
	}

	delete(h.services, key)
	delete(h.destinations, key)
	return nil
}

func (h *fakeHandle) GetServices() ([]*Service, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	result := make([]*Service, 0, len(h.services))
	for _, svc := range h.services {
		result = append(result, cloneService(svc))
	}
	return result, nil
}

func (h *fakeHandle) NewDestination(svc *Service, dst *Destination) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	svcKey := makeFakeServiceKey(svc)
	dstMap, svcExists := h.destinations[svcKey]
	if !svcExists {
		return fmt.Errorf("service %s:%d not found", svc.Address, svc.Port)
	}

	dstKey := makeFakeDestinationKey(dst)
	if _, exists := dstMap[dstKey]; exists {
		return fmt.Errorf("destination %s:%d already exists in service %s:%d",
			dst.Address, dst.Port, svc.Address, svc.Port)
	}

	dstMap[dstKey] = cloneDestination(dst)
	return nil
}

func (h *fakeHandle) UpdateDestination(svc *Service, dst *Destination) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	svcKey := makeFakeServiceKey(svc)
	dstMap, svcExists := h.destinations[svcKey]
	if !svcExists {
		return fmt.Errorf("service %s:%d not found", svc.Address, svc.Port)
	}

	dstKey := makeFakeDestinationKey(dst)
	if _, exists := dstMap[dstKey]; !exists {
		return fmt.Errorf("destination %s:%d not found in service %s:%d",
			dst.Address, dst.Port, svc.Address, svc.Port)
	}

	dstMap[dstKey] = cloneDestination(dst)
	return nil
}

func (h *fakeHandle) DelDestination(svc *Service, dst *Destination) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	svcKey := makeFakeServiceKey(svc)
	dstMap, svcExists := h.destinations[svcKey]
	if !svcExists {
		return fmt.Errorf("service %s:%d not found", svc.Address, svc.Port)
	}

	dstKey := makeFakeDestinationKey(dst)
	if _, exists := dstMap[dstKey]; !exists {
		return fmt.Errorf("destination %s:%d not found in service %s:%d",
			dst.Address, dst.Port, svc.Address, svc.Port)
	}

	delete(dstMap, dstKey)
	return nil
}

func (h *fakeHandle) GetDestinations(svc *Service) ([]*Destination, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	svcKey := makeFakeServiceKey(svc)
	dstMap, svcExists := h.destinations[svcKey]
	if !svcExists {
		return nil, fmt.Errorf("service %s:%d not found", svc.Address, svc.Port)
	}

	result := make([]*Destination, 0, len(dstMap))
	for _, dst := range dstMap {
		result = append(result, cloneDestination(dst))
	}
	return result, nil
}

func (h *fakeHandle) Flush() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.services = make(map[fakeServiceKey]*Service)
	h.destinations = make(map[fakeServiceKey]map[fakeDestinationKey]*Destination)
	return nil
}

// cloneService creates a deep copy of a Service.
func cloneService(svc *Service) *Service {
	return &Service{
		Address:       cloneIP(svc.Address),
		Protocol:      svc.Protocol,
		Port:          svc.Port,
		FWMark:        svc.FWMark,
		SchedName:     svc.SchedName,
		Flags:         svc.Flags,
		Timeout:       svc.Timeout,
		Netmask:       svc.Netmask,
		AddressFamily: svc.AddressFamily,
		PEName:        svc.PEName,
		Stats:         svc.Stats,
	}
}

// cloneDestination creates a deep copy of a Destination.
func cloneDestination(dst *Destination) *Destination {
	return &Destination{
		Address:             cloneIP(dst.Address),
		Port:                dst.Port,
		Weight:              dst.Weight,
		ConnectionFlags:     dst.ConnectionFlags,
		AddressFamily:       dst.AddressFamily,
		UpperThreshold:      dst.UpperThreshold,
		LowerThreshold:      dst.LowerThreshold,
		ActiveConnections:   dst.ActiveConnections,
		InactiveConnections: dst.InactiveConnections,
		Stats:               dst.Stats,
	}
}
