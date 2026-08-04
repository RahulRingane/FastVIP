package lvs

import (
	"net"
	"time"
)

// Service defines an IPVS service in its entirety.
type Service struct {
	SchedName     string
	PEName        string
	Address       net.IP
	Stats         SvcStats
	FWMark        uint32
	Flags         uint32
	Timeout       uint32
	Netmask       uint32
	Protocol      uint16
	Port          uint16
	AddressFamily uint16
}

// SvcStats defines IPVS service statistics.
type SvcStats struct {
	BytesIn     uint64
	BytesOut    uint64
	Connections uint32
	PacketsIn   uint32
	PacketsOut  uint32
	CPS         uint32
	BPSOut      uint32
	PPSIn       uint32
	PPSOut      uint32
	BPSIn       uint32
}

// Destination defines an IPVS destination (real server) in its entirety.
type Destination struct {
	Address             net.IP
	Stats               DstStats
	Weight              int
	ActiveConnections   int
	InactiveConnections int
	ConnectionFlags     uint32
	UpperThreshold      uint32
	LowerThreshold      uint32
	Port                uint16
	AddressFamily       uint16
}

// DstStats defines IPVS destination (real server) statistics.
type DstStats SvcStats

// Config defines IPVS timeout configuration.
type IPVSConfig struct {
	TimeoutTCP    time.Duration
	TimeoutTCPFin time.Duration
	TimeoutUDP    time.Duration
}

// cloneIP returns a copy of the given IP to avoid shared slice references.
func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

// Destination forwarding method constants.
const (
	ConnectionFlagFwdMask     = 0x0007
	ConnectionFlagMasq        = 0x0000
	ConnectionFlagLocalNode   = 0x0001
	ConnectionFlagTunnel      = 0x0002
	ConnectionFlagDirectRoute = 0x0003
)

// Scheduling algorithm constants.
const (
	RoundRobin              = "rr"
	LeastConnection         = "lc"
	DestinationHashing      = "dh"
	SourceHashing           = "sh"
	WeightedRoundRobin      = "wrr"
	WeightedLeastConnection = "wlc"
)

// Connection forwarding method constants (aliases).
const (
	ConnFwdMask        = 0x0007
	ConnFwdMasq        = 0x0000
	ConnFwdLocalNode   = 0x0001
	ConnFwdTunnel      = 0x0002
	ConnFwdDirectRoute = 0x0003
	ConnFwdBypass      = 0x0004
)
