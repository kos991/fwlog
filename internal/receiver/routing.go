package receiver

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"fwlog/internal/model"
)

type endpointKey struct {
	Protocol string
	Host     string
	Port     int
}

type route struct {
	Source  model.LogSource
	Network *net.IPNet
	Prefix  int
	Exact   bool
}

type routeTable struct {
	exact    map[string]route
	networks []route
	catchAll *route
}

func buildRouteTable(sources []model.LogSource) (routeTable, error) {
	table := routeTable{exact: make(map[string]route)}
	seenNetworks := make(map[string]struct{})

	for _, source := range sources {
		clientIP := strings.TrimSpace(source.ClientIP)
		if clientIP == "" {
			if table.catchAll != nil {
				return routeTable{}, fmt.Errorf("重复的全匹配客户端规则")
			}
			candidate := route{Source: source}
			table.catchAll = &candidate
			continue
		}

		if parsed := net.ParseIP(clientIP); parsed != nil {
			key := parsed.String()
			if _, exists := table.exact[key]; exists {
				return routeTable{}, fmt.Errorf("重复的客户端 IP %q", key)
			}
			table.exact[key] = route{Source: source, Exact: true}
			continue
		}

		_, network, err := net.ParseCIDR(clientIP)
		if err != nil {
			return routeTable{}, fmt.Errorf("客户端 IP/网段 %q 无效", clientIP)
		}
		key := network.String()
		if _, exists := seenNetworks[key]; exists {
			return routeTable{}, fmt.Errorf("重复的客户端网段 %q", key)
		}
		seenNetworks[key] = struct{}{}
		prefix, _ := network.Mask.Size()
		table.networks = append(table.networks, route{
			Source:  source,
			Network: network,
			Prefix:  prefix,
		})
	}

	sort.SliceStable(table.networks, func(i, j int) bool {
		return table.networks[i].Prefix > table.networks[j].Prefix
	})
	return table, nil
}

func (t routeTable) Match(ip net.IP) (model.LogSource, bool) {
	if ip == nil {
		return model.LogSource{}, false
	}
	if matched, ok := t.exact[ip.String()]; ok {
		return matched.Source, true
	}
	for _, candidate := range t.networks {
		if candidate.Network.Contains(ip) {
			return candidate.Source, true
		}
	}
	if t.catchAll != nil {
		return t.catchAll.Source, true
	}
	return model.LogSource{}, false
}
