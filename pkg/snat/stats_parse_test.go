package snat

import "testing"

func TestParseSNATStatsRowParsesFoldedRuleFields(t *testing.T) {
	stat := []string{
		"42",
		"1024",
		"SNAT",
		"tcp",
		"--",
		"*",
		"*",
		"0.0.0.0/0",
		"10.0.0.3",
		"tcp dpt:8080 to:192.168.1.1",
	}

	ruleKey, stats, ok := parseSNATStatsRow(stat)
	if !ok {
		t.Fatal("expected folded SNAT stats row to be parsed")
	}
	if ruleKey != "10.0.0.3:8080/tcp" {
		t.Fatalf("expected rule key 10.0.0.3:8080/tcp, got %q", ruleKey)
	}
	if stats.Packets != 42 || stats.Bytes != 1024 {
		t.Fatalf("expected stats {Packets:42 Bytes:1024}, got %+v", stats)
	}
}

func TestParseSNATStatsRowParsesSplitDPortToken(t *testing.T) {
	stat := []string{
		"7",
		"512",
		"MASQUERADE",
		"udp",
		"--",
		"*",
		"*",
		"0.0.0.0/0",
		"10.0.0.4",
		"udp",
		"dpt:",
		"5353",
		"to:192.168.1.2",
	}

	ruleKey, stats, ok := parseSNATStatsRow(stat)
	if !ok {
		t.Fatal("expected split SNAT stats row to be parsed")
	}
	if ruleKey != "10.0.0.4:5353/udp" {
		t.Fatalf("expected rule key 10.0.0.4:5353/udp, got %q", ruleKey)
	}
	if stats.Packets != 7 || stats.Bytes != 512 {
		t.Fatalf("expected stats {Packets:7 Bytes:512}, got %+v", stats)
	}
}
