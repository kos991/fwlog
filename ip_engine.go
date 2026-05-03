package main

import (
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

type IPTag struct {
	IP       string `json:"ip"`
	Label    string `json:"label"`
	Location string `json:"location"`
	IsManual bool   `json:"is_manual"`
}

type IPEngine struct {
	overrides map[string]IPTag
	segments  []networkSegment
	geoDB     *geoip2.Reader
	mu        sync.RWMutex
}

type networkSegment struct {
	Network *net.IPNet
	Label   string
}

func NewIPEngine() *IPEngine {
	engine := &IPEngine{overrides: make(map[string]IPTag), segments: []networkSegment{}}
	engine.AddSegment("172.18.0.0/17", "政务网私有段")
	engine.AddSegment("172.28.128.0/19", "政务网私有段")
	engine.AddSegment("2.0.0.0/8", "政务网私有段")
	return engine
}
func (e *IPEngine) AddSegment(cidr, label string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.segments = append(e.segments, networkSegment{Network: ipnet, Label: label})
	return nil
}
func (e *IPEngine) AddOverride(ip, label, location string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.overrides[ip] = IPTag{IP: ip, Label: label, Location: location, IsManual: true}
}
func (e *IPEngine) LoadGeoDB(filePath string) error {
	db, err := geoip2.Open(filePath)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.geoDB != nil {
		e.geoDB.Close()
	}
	e.geoDB = db
	return nil
}

func (e *IPEngine) GetTag(ipStr string) IPTag {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. 最高优先级：手动覆盖
	if tag, ok := e.overrides[ipStr]; ok {
		return tag
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return IPTag{IP: ipStr, Label: "非法 IP", Location: "未知"}
	}

	// 2. 第二优先级：自定义网段匹配
	for _, seg := range e.segments {
		if seg.Network.Contains(ip) {
			return IPTag{IP: ipStr, Label: seg.Label, Location: "内网"}
		}
	}

	// 3. 第三优先级：私有地址/局域网判断
	if ip.IsPrivate() {
		return IPTag{IP: ipStr, Label: "局域网", Location: "内网"}
	}

	// 4. 最终兜底：公网 (GeoIP)
	location := "未知公网"
	if e.geoDB != nil {
		record, err := e.geoDB.City(ip)
		if err == nil {
			country := record.Country.Names["zh-CN"]
			if country == "" {
				country = record.Country.Names["en"]
			}
			city := record.City.Names["zh-CN"]
			if city == "" {
				city = record.City.Names["en"]
			}
			if country != "" {
				location = country
				if city != "" {
					location += "·" + city
				}
			}
		}
	}

	return IPTag{IP: ipStr, Label: "公网 IP", Location: location}
}

func (e *IPEngine) LoadCustomMap(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	for _, record := range records {
		if len(record) >= 3 {
			e.AddOverride(record[0], record[1], record[2])
		}
	}
	return nil
}
