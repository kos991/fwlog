package threatintel

import "net/http"

func DefaultAdapters(client *http.Client) map[Provider]Adapter {
	return map[Provider]Adapter{
		ProviderThreatBook: NewThreatBookAdapter(client, threatBookDefaultEndpoint),
		ProviderNSFocus:    NewNSFocusAdapter(client, nsfocusDefaultEndpoint, nil),
		ProviderQianxin:    NewQianxinAdapter(client, qianxinDefaultEndpoint),
		ProviderTencent:    NewTencentAdapter(client, tencentDefaultEndpoint),
	}
}
