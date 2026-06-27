package mail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
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
	// Parse and classify before doing anything with the value. A string that
	// is not a valid IP, or that is not a globally-routable unicast address,
	// never reaches the outbound request — this both avoids leaking internal
	// topology to the third-party service and closes the SSRF surface (the
	// caller-influenced IP, which on a misconfigured trusted-proxy setup can be
	// attacker-supplied, can no longer steer the request).
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "Unknown"
	}
	if !isGlobalUnicast(addr) {
		return "Private/Local Network"
	}

	// NOTE: ip-api.com's free endpoint is HTTP-only (HTTPS requires a paid
	// plan), so this lookup still travels in cleartext and discloses the
	// end-user IP to a third party — an accepted trade-off for the free geo
	// data. addr.String() re-serialises the parsed address, so the URL carries
	// only canonical IP characters; there is no path/query injection vector.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + addr.String())
	if err != nil {
		return "Unknown"
	}
	defer func() { _ = resp.Body.Close() }()

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

// isGlobalUnicast reports whether addr is a globally-routable unicast address
// worth resolving. It rejects every non-routable or internal class: RFC 1918
// private and IPv6 ULA (IsPrivate), loopback, link-local unicast/multicast,
// any multicast, the unspecified address, and — which the stdlib helpers do
// NOT cover — the RFC 6598 CGNAT shared range 100.64.0.0/10.
func isGlobalUnicast(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	if addr.Is4() {
		b := addr.As4()
		if b[0] == 100 && b[1] >= 64 && b[1] <= 127 { // 100.64.0.0/10 (CGNAT)
			return false
		}
	}
	return true
}
