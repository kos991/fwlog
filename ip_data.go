package main

import "time"

type IPDataStatus struct {
	Loaded        bool      `json:"loaded"`
	CustomMapPath string    `json:"custom_ip_map_path"`
	GeoIPDBPath   string    `json:"geoip_db_path"`
	IPMapEnabled  bool      `json:"ip_map_enabled"`
	GeoIPEnabled  bool      `json:"geoip_enabled"`
	UpdatedAt     time.Time `json:"updated_at"`
	Error         string    `json:"error"`
}

func ReloadIPEngine(cfg Config, old *IPEngine) (*IPEngine, IPDataStatus) {
	status := IPDataStatus{
		CustomMapPath: cfg.CustomIPMapPath,
		GeoIPDBPath:   cfg.GeoIPDBPath,
		IPMapEnabled:  cfg.IPMapEnabled,
		GeoIPEnabled:  cfg.GeoIPEnabled,
		UpdatedAt:     time.Now(),
	}

	next := NewIPEngine()
	if cfg.IPMapEnabled {
		if err := next.LoadCustomMap(cfg.CustomIPMapPath); err != nil {
			status.Error = err.Error()
			return oldOrNewEngine(old, next), status
		}
	}
	if cfg.GeoIPEnabled {
		if err := next.LoadGeoDB(cfg.GeoIPDBPath); err != nil {
			status.Error = err.Error()
			return oldOrNewEngine(old, next), status
		}
	}

	status.Loaded = true
	return next, status
}

func oldOrNewEngine(old, next *IPEngine) *IPEngine {
	if old != nil {
		return old
	}
	return next
}
