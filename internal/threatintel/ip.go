package threatintel

import "net/netip"

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
		!addr.IsPrivate() &&
		!addr.IsLoopback() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}
