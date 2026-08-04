//go:build integration

package lvs

import (
	mobyipvs "github.com/moby/ipvs"
)

// linuxHandle wraps the real moby/ipvs Handle for Linux systems.
type linuxHandle struct {
	handle *mobyipvs.Handle
}

// NewIPVSHandle creates a real IPVS handle via netlink on Linux.
func NewIPVSHandle(path string) (IPVSHandle, error) {
	handle, err := mobyipvs.New(path)
	if err != nil {
		return nil, err
	}
	return &linuxHandle{handle: handle}, nil
}

func (h *linuxHandle) Close() {
	h.handle.Close()
}

func (h *linuxHandle) NewService(svc *Service) error {
	return h.handle.NewService(toMobyService(svc))
}

func (h *linuxHandle) UpdateService(svc *Service) error {
	return h.handle.UpdateService(toMobyService(svc))
}

func (h *linuxHandle) DelService(svc *Service) error {
	return h.handle.DelService(toMobyService(svc))
}

func (h *linuxHandle) GetServices() ([]*Service, error) {
	mobySvcs, err := h.handle.GetServices()
	if err != nil {
		return nil, err
	}
	services := make([]*Service, len(mobySvcs))
	for i, ms := range mobySvcs {
		services[i] = fromMobyService(ms)
	}
	return services, nil
}

func (h *linuxHandle) NewDestination(svc *Service, dst *Destination) error {
	return h.handle.NewDestination(toMobyService(svc), toMobyDestination(dst))
}

func (h *linuxHandle) UpdateDestination(svc *Service, dst *Destination) error {
	return h.handle.UpdateDestination(toMobyService(svc), toMobyDestination(dst))
}

func (h *linuxHandle) DelDestination(svc *Service, dst *Destination) error {
	return h.handle.DelDestination(toMobyService(svc), toMobyDestination(dst))
}

func (h *linuxHandle) GetDestinations(svc *Service) ([]*Destination, error) {
	mobyDsts, err := h.handle.GetDestinations(toMobyService(svc))
	if err != nil {
		return nil, err
	}
	destinations := make([]*Destination, len(mobyDsts))
	for i, md := range mobyDsts {
		destinations[i] = fromMobyDestination(md)
	}
	return destinations, nil
}

func (h *linuxHandle) Flush() error {
	return h.handle.Flush()
}

// toMobyService converts the local Service type to moby/ipvs Service.
func toMobyService(svc *Service) *mobyipvs.Service {
	return &mobyipvs.Service{
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
	}
}

// fromMobyService converts a moby/ipvs Service to the local Service type.
func fromMobyService(ms *mobyipvs.Service) *Service {
	return &Service{
		Address:       cloneIP(ms.Address),
		Protocol:      ms.Protocol,
		Port:          ms.Port,
		FWMark:        ms.FWMark,
		SchedName:     ms.SchedName,
		Flags:         ms.Flags,
		Timeout:       ms.Timeout,
		Netmask:       ms.Netmask,
		AddressFamily: ms.AddressFamily,
		PEName:        ms.PEName,
		Stats: SvcStats{
			Connections: ms.Stats.Connections,
			PacketsIn:   ms.Stats.PacketsIn,
			PacketsOut:  ms.Stats.PacketsOut,
			BytesIn:     ms.Stats.BytesIn,
			BytesOut:    ms.Stats.BytesOut,
			CPS:         ms.Stats.CPS,
			BPSOut:      ms.Stats.BPSOut,
			PPSIn:       ms.Stats.PPSIn,
			PPSOut:      ms.Stats.PPSOut,
			BPSIn:       ms.Stats.BPSIn,
		},
	}
}

// toMobyDestination converts the local Destination type to moby/ipvs Destination.
func toMobyDestination(dst *Destination) *mobyipvs.Destination {
	return &mobyipvs.Destination{
		Address:         cloneIP(dst.Address),
		Port:            dst.Port,
		Weight:          dst.Weight,
		ConnectionFlags: dst.ConnectionFlags,
		AddressFamily:   dst.AddressFamily,
		UpperThreshold:  dst.UpperThreshold,
		LowerThreshold:  dst.LowerThreshold,
	}
}

// fromMobyDestination converts a moby/ipvs Destination to the local Destination type.
func fromMobyDestination(md *mobyipvs.Destination) *Destination {
	return &Destination{
		Address:             cloneIP(md.Address),
		Port:                md.Port,
		Weight:              md.Weight,
		ConnectionFlags:     md.ConnectionFlags,
		AddressFamily:       md.AddressFamily,
		UpperThreshold:      md.UpperThreshold,
		LowerThreshold:      md.LowerThreshold,
		ActiveConnections:   md.ActiveConnections,
		InactiveConnections: md.InactiveConnections,
		Stats: DstStats{
			Connections: md.Stats.Connections,
			PacketsIn:   md.Stats.PacketsIn,
			PacketsOut:  md.Stats.PacketsOut,
			BytesIn:     md.Stats.BytesIn,
			BytesOut:    md.Stats.BytesOut,
			CPS:         md.Stats.CPS,
			BPSOut:      md.Stats.BPSOut,
			PPSIn:       md.Stats.PPSIn,
			PPSOut:      md.Stats.PPSOut,
			BPSIn:       md.Stats.BPSIn,
		},
	}
}
