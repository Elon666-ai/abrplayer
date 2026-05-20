package utils

import (
	"log"
	"net"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/oschwald/geoip2-golang"
)

var (
	geoDB     *geoip2.Reader
	initOnce  sync.Once
	dbLoadErr error
	ipCache   *lru.Cache
)

func init() {
	// create an in-memory LRU cache for IP->location (size tunable)
	c, err := lru.New(10000)
	if err != nil {
		log.Printf("⚠️ failed to create ip cache: %v", err)
		ipCache = nil
		return
	}
	ipCache = c
}

// InitGeoIP 初始化 GeoIP 数据库. load to RAM
func InitGeoIP(dbPath string) error {
	initOnce.Do(func() {
		geoDB, dbLoadErr = geoip2.Open(dbPath)
		if dbLoadErr != nil {
			log.Printf("⚠️ GeoIP database not loaded: %v", dbLoadErr)
		} else {
			log.Printf("✅ GeoIP database loaded: %s", dbPath)
		}
	})
	return dbLoadErr
}

// IPToLocation 将 IP 解析为 "国家/城市" 字符串. time cost = 0.1 ms
func IPToLocation(ipStr string) string {
	if ipStr == "" || geoDB == nil {
		return "Unknown"
	}

	// check cache
	if ipCache != nil {
		if v, ok := ipCache.Get(ipStr); ok {
			return v.(string)
		}
	}

	// simple local checks
	if ipStr == "127.0.0.1" || ipStr == "::1" || strings.HasPrefix(ipStr, "192.168.") || strings.HasPrefix(ipStr, "10.") || strings.HasPrefix(ipStr, "169.254.") {
		ipCacheAdd(ipStr, "Local")
		return "Local"
	}
	// common 172.16.0.0-172.31.255.255
	if strings.HasPrefix(ipStr, "172.") {
		// quick check second octet
		parts := strings.Split(ipStr, ".")
		if len(parts) >= 2 {
			if s := parts[1]; s >= "16" && s <= "31" {
				ipCacheAdd(ipStr, "Local")
				return "Local"
			}
		}
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		ipCacheAdd(ipStr, "Invalid IP")
		return "Invalid IP"
	}

	record, err := geoDB.City(ip)
	if err != nil {
		ipCacheAdd(ipStr, "Unknown")
		return "Unknown"
	}

	country := record.Country.Names["en"]
	city := record.City.Names["en"]

	location := country
	if city != "" {
		location = country + "/" + city
	}

	if location == "" {
		location = "Unknown"
	}
	ipCacheAdd(ipStr, location)
	return location
}

func ipCacheAdd(k, v string) {
	if ipCache == nil {
		return
	}
	ipCache.Add(k, v)
}
