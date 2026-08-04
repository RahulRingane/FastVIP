package lvs

import (
	"net"
	"syscall"
	"testing"

	"github.com/RahulRingane/FastVIP/pkg/config"
)

// --- Protocol conversion tests ---

func TestProtocolFromString_TCP(t *testing.T) {
	proto, err := protocolFromString("tcp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proto != syscall.IPPROTO_TCP {
		t.Errorf("expected IPPROTO_TCP (%d), got %d", syscall.IPPROTO_TCP, proto)
	}
}

func TestProtocolFromString_UDP(t *testing.T) {
	proto, err := protocolFromString("udp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proto != syscall.IPPROTO_UDP {
		t.Errorf("expected IPPROTO_UDP (%d), got %d", syscall.IPPROTO_UDP, proto)
	}
}

func TestProtocolFromString_Unknown(t *testing.T) {
	_, err := protocolFromString("unknown")
	if err == nil {
		t.Fatal("expected error for unknown protocol, got nil")
	}
}

func TestProtocolToString_TCP(t *testing.T) {
	result := protocolToString(syscall.IPPROTO_TCP)
	if result != "tcp" {
		t.Errorf("expected 'tcp', got %q", result)
	}
}

func TestProtocolToString_UDP(t *testing.T) {
	result := protocolToString(syscall.IPPROTO_UDP)
	if result != "udp" {
		t.Errorf("expected 'udp', got %q", result)
	}
}

func TestProtocolToString_Unknown(t *testing.T) {
	result := protocolToString(999)
	expected := "unknown(999)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// --- Address family tests ---

func TestAddressFamilyFromIP_IPv4(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	family := addressFamilyFromIP(ip)
	if family != syscall.AF_INET {
		t.Errorf("expected AF_INET (%d), got %d", syscall.AF_INET, family)
	}
}

func TestAddressFamilyFromIP_IPv6(t *testing.T) {
	ip := net.ParseIP("::1")
	family := addressFamilyFromIP(ip)
	if family != syscall.AF_INET6 {
		t.Errorf("expected AF_INET6 (%d), got %d", syscall.AF_INET6, family)
	}
}

func TestNetmaskFromFamily_IPv4(t *testing.T) {
	mask := netmaskFromFamily(syscall.AF_INET)
	if mask != 0xFFFFFFFF {
		t.Errorf("expected 0xFFFFFFFF, got 0x%X", mask)
	}
}

func TestNetmaskFromFamily_IPv6(t *testing.T) {
	mask := netmaskFromFamily(syscall.AF_INET6)
	if mask != 128 {
		t.Errorf("expected 128, got %d", mask)
	}
}

// --- ServiceKey tests ---

func TestServiceKeyFromConfig_Valid(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:   "10.0.0.1:80",
		Protocol: "tcp",
	}
	key, err := ServiceKeyFromConfig(svcCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.Address != "10.0.0.1" {
		t.Errorf("expected address '10.0.0.1', got %q", key.Address)
	}
	if key.Port != 80 {
		t.Errorf("expected port 80, got %d", key.Port)
	}
	if key.Protocol != syscall.IPPROTO_TCP {
		t.Errorf("expected protocol IPPROTO_TCP, got %d", key.Protocol)
	}
}

func TestServiceKeyFromConfig_InvalidListen(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:   "not-valid",
		Protocol: "tcp",
	}
	_, err := ServiceKeyFromConfig(svcCfg)
	if err == nil {
		t.Fatal("expected error for invalid listen address, got nil")
	}
}

func TestServiceKeyFromConfig_InvalidPort(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:   "10.0.0.1:abc",
		Protocol: "tcp",
	}
	_, err := ServiceKeyFromConfig(svcCfg)
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestServiceKeyFromConfig_InvalidProtocol(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:   "10.0.0.1:80",
		Protocol: "invalid",
	}
	_, err := ServiceKeyFromConfig(svcCfg)
	if err == nil {
		t.Fatal("expected error for invalid protocol, got nil")
	}
}

func TestServiceKeyFromIPVS(t *testing.T) {
	svc := &Service{
		Address:  net.ParseIP("10.0.0.1"),
		Port:     80,
		Protocol: syscall.IPPROTO_TCP,
	}
	key := ServiceKeyFromIPVS(svc)
	if key.Address != "10.0.0.1" {
		t.Errorf("expected address '10.0.0.1', got %q", key.Address)
	}
	if key.Port != 80 {
		t.Errorf("expected port 80, got %d", key.Port)
	}
	if key.Protocol != syscall.IPPROTO_TCP {
		t.Errorf("expected protocol IPPROTO_TCP, got %d", key.Protocol)
	}
}

func TestServiceKey_String(t *testing.T) {
	key := ServiceKey{
		Address:  "10.0.0.1",
		Port:     80,
		Protocol: syscall.IPPROTO_TCP,
	}
	expected := "10.0.0.1:80/tcp"
	if key.String() != expected {
		t.Errorf("expected %q, got %q", expected, key.String())
	}
}

// --- DestinationKey tests ---

func TestDestinationKeyFromIPVS(t *testing.T) {
	dst := &Destination{
		Address: net.ParseIP("192.168.1.1"),
		Port:    8080,
	}
	key := DestinationKeyFromIPVS(dst)
	if key.Address != "192.168.1.1" {
		t.Errorf("expected address '192.168.1.1', got %q", key.Address)
	}
	if key.Port != 8080 {
		t.Errorf("expected port 8080, got %d", key.Port)
	}
}

func TestDestinationKey_String(t *testing.T) {
	key := DestinationKey{
		Address: "192.168.1.1",
		Port:    8080,
	}
	expected := "192.168.1.1:8080"
	if key.String() != expected {
		t.Errorf("expected %q, got %q", expected, key.String())
	}
}

// --- ConfigToIPVS conversion tests ---

func TestConfigToIPVSService_ValidTCP(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:    "10.0.0.1:80",
		Protocol:  "tcp",
		Scheduler: "wrr",
	}
	svc, err := ConfigToIPVSService(svcCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !svc.Address.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected address 10.0.0.1, got %s", svc.Address)
	}
	if svc.Port != 80 {
		t.Errorf("expected port 80, got %d", svc.Port)
	}
	if svc.Protocol != syscall.IPPROTO_TCP {
		t.Errorf("expected protocol IPPROTO_TCP, got %d", svc.Protocol)
	}
	if svc.SchedName != "wrr" {
		t.Errorf("expected scheduler 'wrr', got %q", svc.SchedName)
	}
	if svc.AddressFamily != syscall.AF_INET {
		t.Errorf("expected AF_INET, got %d", svc.AddressFamily)
	}
	if svc.Netmask != 0xFFFFFFFF {
		t.Errorf("expected netmask 0xFFFFFFFF, got 0x%X", svc.Netmask)
	}
}

func TestConfigToIPVSService_InvalidListen(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:   "bad-address",
		Protocol: "tcp",
	}
	_, err := ConfigToIPVSService(svcCfg)
	if err == nil {
		t.Fatal("expected error for invalid listen address, got nil")
	}
}

func TestConfigToIPVSService_InvalidIP(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:   "not-an-ip:80",
		Protocol: "tcp",
	}
	_, err := ConfigToIPVSService(svcCfg)
	if err == nil {
		t.Fatal("expected error for invalid IP, got nil")
	}
}

func TestConfigToIPVSService_IPv6(t *testing.T) {
	svcCfg := config.ServiceConfig{
		Listen:    "[::1]:80",
		Protocol:  "tcp",
		Scheduler: "rr",
	}
	svc, err := ConfigToIPVSService(svcCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.AddressFamily != syscall.AF_INET6 {
		t.Errorf("expected AF_INET6, got %d", svc.AddressFamily)
	}
	if svc.Netmask != 128 {
		t.Errorf("expected netmask 128 for IPv6, got %d", svc.Netmask)
	}
}

func TestConfigToIPVSDestination_Valid(t *testing.T) {
	backendCfg := config.BackendConfig{
		Address: "192.168.1.10:8080",
		Weight:  5,
	}
	dst, err := ConfigToIPVSDestination(backendCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dst.Address.Equal(net.ParseIP("192.168.1.10")) {
		t.Errorf("expected address 192.168.1.10, got %s", dst.Address)
	}
	if dst.Port != 8080 {
		t.Errorf("expected port 8080, got %d", dst.Port)
	}
	if dst.Weight != 5 {
		t.Errorf("expected weight 5, got %d", dst.Weight)
	}
	if dst.ConnectionFlags != ConnectionFlagMasq {
		t.Errorf("expected ConnectionFlagMasq, got %d", dst.ConnectionFlags)
	}
	if dst.AddressFamily != syscall.AF_INET {
		t.Errorf("expected AF_INET, got %d", dst.AddressFamily)
	}
}

func TestConfigToIPVSDestination_InvalidAddress(t *testing.T) {
	backendCfg := config.BackendConfig{
		Address: "not-valid",
		Weight:  1,
	}
	_, err := ConfigToIPVSDestination(backendCfg)
	if err == nil {
		t.Fatal("expected error for invalid backend address, got nil")
	}
}

func TestConfigToIPVSDestination_InvalidIP(t *testing.T) {
	backendCfg := config.BackendConfig{
		Address: "bad-ip:8080",
		Weight:  1,
	}
	_, err := ConfigToIPVSDestination(backendCfg)
	if err == nil {
		t.Fatal("expected error for invalid backend IP, got nil")
	}
}
