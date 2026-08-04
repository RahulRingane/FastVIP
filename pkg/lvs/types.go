package lvs

import (
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/RahulRingane/FastVIP/pkg/config"
)

// ServiceKey uniquely identifies an IPVS virtual service.
type ServiceKey struct {
	Address  string
	Port     uint16
	Protocol uint16
}

// String returns a human-readable representation of the ServiceKey.
func (k ServiceKey) String() string {
	return fmt.Sprintf("%s:%d/%s", k.Address, k.Port, protocolToString(k.Protocol))
}

// protocolToString converts a protocol number to its string name.
func protocolToString(protocol uint16) string {
	switch protocol {
	case syscall.IPPROTO_TCP:
		return "tcp"
	case syscall.IPPROTO_UDP:
		return "udp"
	default:
		return fmt.Sprintf("unknown(%d)", protocol)
	}
}

// DestinationKey uniquely identifies an IPVS destination within a service.
type DestinationKey struct {
	Address string
	Port    uint16
}

// String returns a human-readable representation of the DestinationKey.
func (k DestinationKey) String() string {
	return fmt.Sprintf("%s:%d", k.Address, k.Port)
}

// protocolFromString converts a protocol string to its syscall constant.
func protocolFromString(protocol string) (uint16, error) {
	switch protocol {
	case "tcp":
		return syscall.IPPROTO_TCP, nil
	case "udp":
		return syscall.IPPROTO_UDP, nil
	default:
		return 0, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

// addressFamilyFromIP determines the address family (IPv4 or IPv6) from an IP address.
func addressFamilyFromIP(ipAddress net.IP) uint16 {
	if ipAddress.To4() != nil {
		return syscall.AF_INET
	}
	return syscall.AF_INET6
}

// netmaskFromFamily returns the appropriate netmask for the given address family.
func netmaskFromFamily(family uint16) uint32 {
	if family == syscall.AF_INET {
		return 0xFFFFFFFF
	}
	return 128
}

// ServiceKeyFromConfig generates a ServiceKey from a ServiceConfig.
func ServiceKeyFromConfig(svcCfg config.ServiceConfig) (ServiceKey, error) {
	host, portStr, err := net.SplitHostPort(svcCfg.Listen)
	if err != nil {
		return ServiceKey{}, fmt.Errorf("invalid listen address %q: %w", svcCfg.Listen, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return ServiceKey{}, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	protocol, err := protocolFromString(svcCfg.Protocol)
	if err != nil {
		return ServiceKey{}, err
	}

	return ServiceKey{
		Address:  host,
		Port:     uint16(port),
		Protocol: protocol,
	}, nil
}

// ServiceKeyFromIPVS generates a ServiceKey from a Service.
func ServiceKeyFromIPVS(svc *Service) ServiceKey {
	return ServiceKey{
		Address:  svc.Address.String(),
		Port:     svc.Port,
		Protocol: svc.Protocol,
	}
}

// DestinationKeyFromIPVS generates a DestinationKey from a Destination.
func DestinationKeyFromIPVS(dst *Destination) DestinationKey {
	return DestinationKey{
		Address: dst.Address.String(),
		Port:    dst.Port,
	}
}

// ConfigToIPVSService converts a ServiceConfig to a Service struct.
func ConfigToIPVSService(svcCfg config.ServiceConfig) (*Service, error) {
	host, portStr, err := net.SplitHostPort(svcCfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address %q: %w", svcCfg.Listen, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	ipAddress := net.ParseIP(host)
	if ipAddress == nil {
		return nil, fmt.Errorf("invalid IP address %q", host)
	}
	if ipv4 := ipAddress.To4(); ipv4 != nil {
		ipAddress = ipv4
	}

	protocol, err := protocolFromString(svcCfg.Protocol)
	if err != nil {
		return nil, err
	}

	family := addressFamilyFromIP(ipAddress)

	return &Service{
		Address:       ipAddress,
		Protocol:      protocol,
		Port:          uint16(port),
		SchedName:     svcCfg.Scheduler,
		AddressFamily: family,
		Netmask:       netmaskFromFamily(family),
	}, nil
}

// ConfigToIPVSDestination converts a BackendConfig to a Destination struct.
func ConfigToIPVSDestination(backendCfg config.BackendConfig) (*Destination, error) {
	host, portStr, err := net.SplitHostPort(backendCfg.Address)
	if err != nil {
		return nil, fmt.Errorf("invalid backend address %q: %w", backendCfg.Address, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	ipAddress := net.ParseIP(host)
	if ipAddress == nil {
		return nil, fmt.Errorf("invalid IP address %q", host)
	}
	if ipv4 := ipAddress.To4(); ipv4 != nil {
		ipAddress = ipv4
	}

	family := addressFamilyFromIP(ipAddress)

	return &Destination{
		Address:         ipAddress,
		Port:            uint16(port),
		Weight:          backendCfg.Weight,
		ConnectionFlags: ConnectionFlagMasq,
		AddressFamily:   family,
	}, nil
}
