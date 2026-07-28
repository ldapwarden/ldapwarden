package api

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestSecureHeaders_AlwaysOn(t *testing.T) {
	handler := secureHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": contentSecurityPolicy,
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// inlineScript matches the theme-bootstrap <script> in web/index.html. It is the
// only inline script the app ships; a second one would need its own hash.
var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// TestContentSecurityPolicy_AllowsThemeBootstrap recomputes the hash of the
// inline theme script from web/index.html and checks the CSP still allows it.
//
// Without this the coupling is invisible: editing the script (or reformatting
// index.html) changes its hash, the browser silently blocks it, and every page
// load flashes the wrong theme — with nothing failing anywhere.
func TestContentSecurityPolicy_AllowsThemeBootstrap(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file path")
	}
	indexPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "web", "index.html")

	html, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read web/index.html: %v", err)
	}

	matches := inlineScript.FindAllSubmatch(html, -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one inline <script> in web/index.html, found %d — each one needs its own hash in the CSP", len(matches))
	}

	// The browser hashes the script's exact text content, byte for byte.
	sum := sha256.Sum256(matches[0][1])
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"

	if themeBootstrapHash != want {
		t.Errorf("themeBootstrapHash = %s, want %s (web/index.html changed; update the constant in security_headers.go)", themeBootstrapHash, want)
	}
	if !strings.Contains(contentSecurityPolicy, "script-src 'self' "+want) {
		t.Errorf("CSP does not allow the theme bootstrap script: %s", contentSecurityPolicy)
	}
}

func TestSecureHeaders_HSTSOnlyOverTLS(t *testing.T) {
	handler := secureHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("plain HTTP omits HSTS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("HSTS unexpectedly set on plain HTTP: %q", got)
		}
	})

	t.Run("TLS sets HSTS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := "max-age=63072000; includeSubDomains"
		if got := rec.Header().Get("Strict-Transport-Security"); got != want {
			t.Errorf("HSTS = %q, want %q", got, want)
		}
	})
}
