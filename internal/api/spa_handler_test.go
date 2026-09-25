package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSPAHandler pins what the bundle handler serves, and above all what it
// refuses: nothing outside the static root is readable, whether reached with
// `..` segments or through a symlink planted inside the bundle.
func TestSPAHandler(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	dist := t.TempDir()
	files := map[string]string{
		"index.html":       "<html>app</html>",
		"assets/app.js":    "console.log('app')",
		"assets/style.css": "body{}",
	}
	for name, body := range files {
		p := filepath.Join(dist, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(secret, filepath.Join(dist, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dist, "leak")); err != nil {
		t.Fatal(err)
	}

	h, err := spaHandler(dist)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		{"existing asset", "/assets/app.js", http.StatusOK, "console.log('app')"},
		{"root", "/", http.StatusOK, "<html>app</html>"},
		{"client-side route", "/users/uid=jdoe", http.StatusOK, "<html>app</html>"},
		{"missing asset", "/assets/missing.js", http.StatusNotFound, ""},
		{"dot-dot traversal", "/../" + filepath.Base(outside) + "/secret.txt", http.StatusNotFound, ""},
		{"symlink escape with extension", "/leak.txt", http.StatusNotFound, ""},
		{"symlink escape without extension", "/leak", http.StatusOK, "<html>app</html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			// Set the path directly: NewRequest would clean `..` away before
			// the handler ever saw it.
			req.URL.Path = tt.path
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			body := rec.Body.String()
			if strings.Contains(body, "top secret") {
				t.Fatalf("served a file outside the static root: %q", body)
			}
			if tt.wantBody != "" && body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
