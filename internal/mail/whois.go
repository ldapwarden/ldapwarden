package mail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ipAPIResponse struct {
	Status      string `json:"status"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	Region      string `json:"region"`
	RegionName  string `json:"regionName"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	Org         string `json:"org"`
}

func GetWhoisInfo(ip string) string {
	// Skip lookup for private/local IPs
	if isPrivateIP(ip) {
		return "Private/Local Network"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://ip-api.com/json/%s", ip))
	if err != nil {
		return "Unknown"
	}
	defer resp.Body.Close()

	var result ipAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "Unknown"
	}

	if result.Status != "success" {
		return "Unknown"
	}

	parts := []string{}
	if result.City != "" {
		parts = append(parts, result.City)
	}
	if result.RegionName != "" {
		parts = append(parts, result.RegionName)
	}
	if result.Country != "" {
		parts = append(parts, result.Country)
	}

	location := strings.Join(parts, ", ")

	if result.Org != "" {
		return fmt.Sprintf("%s (%s)", location, result.Org)
	}
	if result.ISP != "" {
		return fmt.Sprintf("%s (%s)", location, result.ISP)
	}

	return location
}

func isPrivateIP(ip string) bool {
	privateRanges := []string{
		"10.",
		"172.16.", "172.17.", "172.18.", "172.19.",
		"172.20.", "172.21.", "172.22.", "172.23.",
		"172.24.", "172.25.", "172.26.", "172.27.",
		"172.28.", "172.29.", "172.30.", "172.31.",
		"192.168.",
		"127.",
		"::1",
		"fe80:",
	}

	for _, prefix := range privateRanges {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}

	return false
}
