package receiver

import (
	"net"
	"strings"
	"testing"

	"fwlog/internal/model"
)

func TestRouteTablePrefersExactIPThenLongestCIDRThenCatchAll(t *testing.T) {
	table, err := buildRouteTable([]model.LogSource{
		{SourceID: "all", ClientIP: "", SpoolDir: "/all"},
		{SourceID: "network", ClientIP: "10.0.0.0/8", SpoolDir: "/network"},
		{SourceID: "site", ClientIP: "10.20.0.0/16", SpoolDir: "/site"},
		{SourceID: "exact", ClientIP: "10.20.30.40", SpoolDir: "/exact"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for ip, want := range map[string]string{
		"10.20.30.40": "exact",
		"10.20.1.2":   "site",
		"10.9.1.2":    "network",
		"192.0.2.1":   "all",
	} {
		got, ok := table.Match(net.ParseIP(ip))
		if !ok || got.SourceID != want {
			t.Fatalf("Match(%s) = %#v, %v; want %s", ip, got, ok, want)
		}
	}
}

func TestRouteTableReturnsNoMatchWithoutCatchAll(t *testing.T) {
	table, err := buildRouteTable([]model.LogSource{{SourceID: "site", ClientIP: "10.20.0.0/16"}})
	if err != nil {
		t.Fatal(err)
	}

	if got, ok := table.Match(net.ParseIP("192.0.2.1")); ok {
		t.Fatalf("unexpected match: %#v", got)
	}
	if got, ok := table.Match(nil); ok {
		t.Fatalf("nil IP unexpectedly matched: %#v", got)
	}
}

func TestRouteTableRejectsDuplicateRules(t *testing.T) {
	tests := []struct {
		name    string
		routes  []model.LogSource
		message string
	}{
		{
			name: "exact IP",
			routes: []model.LogSource{
				{SourceID: "a", ClientIP: "10.20.30.40"},
				{SourceID: "b", ClientIP: "10.20.30.40"},
			},
			message: "10.20.30.40",
		},
		{
			name: "CIDR",
			routes: []model.LogSource{
				{SourceID: "a", ClientIP: "10.20.0.0/16"},
				{SourceID: "b", ClientIP: "10.20.1.1/16"},
			},
			message: "10.20.0.0/16",
		},
		{
			name: "catch all",
			routes: []model.LogSource{
				{SourceID: "a", ClientIP: ""},
				{SourceID: "b", ClientIP: ""},
			},
			message: "全匹配",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildRouteTable(test.routes)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("buildRouteTable error = %v; want containing %q", err, test.message)
			}
		})
	}
}

func TestRouteTableNormalizesIPv4AndIPv6Addresses(t *testing.T) {
	table, err := buildRouteTable([]model.LogSource{
		{SourceID: "ipv4", ClientIP: "192.0.2.10"},
		{SourceID: "ipv6", ClientIP: "2001:0db8:0000:0000:0000:0000:0000:0001"},
		{SourceID: "ipv6-network", ClientIP: "2001:db8:abcd:1::42/64"},
	})
	if err != nil {
		t.Fatal(err)
	}

	for ip, want := range map[string]string{
		"192.0.2.10":         "ipv4",
		"2001:db8::1":        "ipv6",
		"2001:db8:abcd:1::9": "ipv6-network",
	} {
		got, ok := table.Match(net.ParseIP(ip))
		if !ok || got.SourceID != want {
			t.Fatalf("Match(%s) = %#v, %v; want %s", ip, got, ok, want)
		}
	}
}
