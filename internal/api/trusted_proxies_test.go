package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		out, err := ParseTrustedProxies(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("len=%d, want 0", len(out))
		}
	})
	t.Run("valid", func(t *testing.T) {
		out, err := ParseTrustedProxies([]string{"10.0.0.0/8", "127.0.0.1/32", "::1/128"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 3 {
			t.Errorf("len=%d, want 3", len(out))
		}
	})
	t.Run("invalid CIDR fails fast", func(t *testing.T) {
		_, err := ParseTrustedProxies([]string{"10.0.0.0/8", "not-a-cidr"})
		if err == nil {
			t.Errorf("expected error on invalid CIDR")
		}
	})
	t.Run("blank entries skipped", func(t *testing.T) {
		out, err := ParseTrustedProxies([]string{"10.0.0.0/8", ""})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 {
			t.Errorf("len=%d, want 1", len(out))
		}
	})
}

func TestPeerIsTrusted(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8", "127.0.0.1/32"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{"in 10/8 with port", "10.5.6.7:34567", true},
		{"in 10/8 no port", "10.5.6.7", true},
		{"loopback /32", "127.0.0.1:1234", true},
		{"out of range", "8.8.8.8:443", false},
		{"empty", "", false},
		{"garbage", "not-an-ip", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := peerIsTrusted(tc.remoteAddr, trusted)
			if got != tc.want {
				t.Errorf("peerIsTrusted(%q) = %v, want %v", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

func TestPeerIsTrusted_EmptyListNeverTrusts(t *testing.T) {
	if peerIsTrusted("127.0.0.1:1234", nil) {
		t.Errorf("empty trusted list must reject every peer")
	}
}

// TestTrustedProxyRealIPMiddleware exercises the four interesting cases:
//   - empty trusted list: header ignored, RemoteAddr untouched
//   - trusted peer with a valid header: RemoteAddr rewritten
//   - untrusted peer with a header: RemoteAddr untouched
//   - trusted peer with a bogus header: RemoteAddr untouched
func TestTrustedProxyRealIPMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		trusted    []string // nil = empty trusted list
		remoteAddr string
		header     string
		headerVal  string
		want       string
	}{
		{
			name:       "empty list ignores header",
			trusted:    nil,
			remoteAddr: "10.0.0.5:1234",
			header:     "X-Forwarded-For", headerVal: "1.2.3.4",
			want: "10.0.0.5:1234",
		},
		{
			name:       "trusted peer + XFF rewrites",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			header:     "X-Forwarded-For", headerVal: "1.2.3.4",
			want: "1.2.3.4",
		},
		{
			name:       "trusted peer + X-Real-IP rewrites",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			header:     "X-Real-IP", headerVal: "5.6.7.8",
			want: "5.6.7.8",
		},
		{
			name:       "untrusted peer + XFF untouched",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "8.8.8.8:1234",
			header:     "X-Forwarded-For", headerVal: "1.2.3.4",
			want: "8.8.8.8:1234",
		},
		{
			name:       "trusted peer + bogus XFF untouched",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.5:1234",
			header:     "X-Forwarded-For", headerVal: "not-an-ip",
			want: "10.0.0.5:1234",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tList := []*net.IPNet(nil)
			if tc.trusted != nil {
				parsed, err := ParseTrustedProxies(tc.trusted)
				if err != nil {
					t.Fatalf("ParseTrustedProxies: %v", err)
				}
				tList = parsed
			}

			var seen string
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = r.RemoteAddr
			})

			h := trustedProxyRealIPMiddleware(tList)(next)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			req.Header.Set(tc.header, tc.headerVal)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if seen != tc.want {
				t.Errorf("RemoteAddr after middleware = %q, want %q", seen, tc.want)
			}
		})
	}
}
