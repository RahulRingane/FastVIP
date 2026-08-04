package snat

import (
	"fmt"
	"strings"
)

func parseSNATStatsRow(stat []string) (string, SNATRuleStats, bool) {
	if len(stat) < 9 {
		return "", SNATRuleStats{}, false
	}

	pkts, err := parseUint64(stat[0])
	if err != nil {
		return "", SNATRuleStats{}, false
	}
	bytes, err := parseUint64(stat[1])
	if err != nil {
		return "", SNATRuleStats{}, false
	}

	protocol := stat[3]
	destination := stat[8]
	dport := extractDPort(stat[9:])
	if protocol == "" || destination == "" || dport == "" {
		return "", SNATRuleStats{}, false
	}

	ruleKey := fmt.Sprintf("%s:%s/%s", destination, dport, protocol)
	return ruleKey, SNATRuleStats{
		Packets: pkts,
		Bytes:   bytes,
	}, true
}

func extractDPort(extra []string) string {
	if len(extra) == 0 {
		return ""
	}

	tokens := strings.Fields(strings.Join(extra, " "))
	for i, token := range tokens {
		switch {
		case token == "dpt:" && i+1 < len(tokens):
			return tokens[i+1]
		case strings.HasPrefix(token, "dpt:") && len(token) > len("dpt:"):
			return strings.TrimPrefix(token, "dpt:")
		}
	}

	return ""
}

func parseUint64(s string) (uint64, error) {
	var val uint64
	_, err := fmt.Sscanf(s, "%d", &val)
	return val, err
}
