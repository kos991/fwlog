package threatintel

import "net/netip"

var specialUseIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func NormalizePublicIP(raw string) (string, error) {
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", newServiceError(ErrorInvalidIP, "请输入有效的 IP 地址", err)
	}

	addr = addr.Unmap()
	if addr == netip.MustParseAddr("255.255.255.255") || !isPublicIP(addr) {
		return "", newServiceError(ErrorUnsupportedIP, "仅支持公网 IP 地址", nil)
	}

	return addr.String(), nil
}

func isPublicIP(addr netip.Addr) bool {
	return addr.IsGlobalUnicast() &&
		!isSpecialUseIP(addr) &&
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

func isSpecialUseIP(addr netip.Addr) bool {
	for _, prefix := range specialUseIPPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
