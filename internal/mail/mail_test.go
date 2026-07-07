package mail

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

func TestValidateHeaderValue(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"plain ascii", "alice@example.com", false},
		{"empty", "", false},
		{"unicode body", "Alïce Dupont", false},
		{"tab", "alice\tbob", false},
		{"CR injection", "alice\rBcc: x", true},
		{"LF injection", "alice\nBcc: x", true},
		{"CRLF injection", "alice\r\nBcc: x", true},
		{"NUL injection", "alice\x00", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHeaderValue(tc.value)
			if tc.wantErr && err == nil {
				t.Errorf("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

func TestBuildMessage_RejectsHeaderInjection(t *testing.T) {
	m := NewMailer(&config.MailConfig{From: "noreply@example.org"}, "Example", "https://ldap.example.org")

	cases := []struct {
		name    string
		from    string
		to      string
		subject string
	}{
		{"to has CRLF", "noreply@example.org", "victim@x\r\nBcc: attacker@evil", "ok"},
		{"subject has CR", "noreply@example.org", "victim@x", "Hello\rBcc: attacker"},
		{"subject has LF", "noreply@example.org", "victim@x", "Hello\nattack"},
		{"from has NUL", "noreply\x00@example.org", "victim@x", "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m.config.From = tc.from
			_, err := m.buildMessage(tc.to, tc.subject, "<p>body</p>")
			if !errors.Is(err, errHeaderInjection) {
				t.Errorf("want errHeaderInjection, got %v", err)
			}
		})
	}
}

func TestBuildMessage_AcceptsCleanInputs(t *testing.T) {
	m := NewMailer(&config.MailConfig{From: "noreply@example.org"}, "Example", "https://ldap.example.org")
	msg, err := m.buildMessage("victim@example.org", "Hello world", "<p>body</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(msg)
	if !strings.Contains(got, "To: victim@example.org\r\n") {
		t.Errorf("To header missing or malformed: %q", got)
	}
	if !strings.Contains(got, "Subject: Hello world\r\n") {
		t.Errorf("Subject header missing or malformed: %q", got)
	}
	if !strings.Contains(got, "From: noreply@example.org\r\n") {
		t.Errorf("From header missing or malformed: %q", got)
	}
}

// TestBuildMessage_FromName checks the optional display name is rendered as
// "Name <addr>" in the From header, while a bare address is left untouched.
func TestBuildMessage_FromName(t *testing.T) {
	m := NewMailer(&config.MailConfig{From: "noreply@example.org", FromName: "LDAP Warden"}, "Example", "https://ldap.example.org")
	msg, err := m.buildMessage("victim@example.org", "Hello world", "<p>body</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "From: \"LDAP Warden\" <noreply@example.org>\r\n"; !strings.Contains(string(msg), want) {
		t.Errorf("From header = %q, want it to contain %q", string(msg), want)
	}
}

func TestAuditSubject(t *testing.T) {
	cases := []struct {
		action    string
		typeLabel string
		name      string
		want      string
	}{
		{"user.create", "account", "Romain Besson", "New account created: Romain Besson"},
		{"group.create", "group", "anyware-ext", "New group created: anyware-ext"},
		{"user.update", "account", "Romain Besson", "Modification of Romain Besson"},
		{"user.delete", "account", "Anna De Fina", "Account deleted: Anna De Fina"},
		{"user.lock", "account", "James Hawken", "Account locked: James Hawken"},
		{"user.unlock", "account", "James Hawken", "Account unlocked: James Hawken"},
		{"group.member.add", "group", "anyware-ext", "Membership change: anyware-ext"},
	}
	for _, tc := range cases {
		if got := auditSubject(tc.action, tc.typeLabel, tc.name); got != tc.want {
			t.Errorf("auditSubject(%q, %q, %q) = %q, want %q", tc.action, tc.typeLabel, tc.name, got, tc.want)
		}
	}
}

func TestHumanizeUserAgent(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:152.0) Gecko/20100101 Firefox/152.0":                                                     "Firefox 152 · macOS",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36":                           "Chrome 126 · Windows",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0":             "Edge 126 · Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15":                     "Safari 17 · macOS",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36":                     "Chrome 126 · Android",
		"curl/8.4.0": "curl/8.4.0", // unrecognised → raw fallback
		"":           "",
	}
	for ua, want := range cases {
		if got := humanizeUserAgent(ua); got != want {
			t.Errorf("humanizeUserAgent(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestAuditAccent(t *testing.T) {
	cases := map[string]string{
		"user.create":      "#059669",
		"group.delete":     "#dc2626",
		"user.lock":        "#d97706",
		"user.unlock":      "#0d9488",
		"group.member.add": "#7c3aed",
		"user.update":      "#2563eb",
	}
	for action, want := range cases {
		if got := auditAccent(action); got != want {
			t.Errorf("auditAccent(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestRDNValue(t *testing.T) {
	cases := map[string]string{
		"uid=jdoe,ou=people,dc=example,dc=org": "jdoe",
		"cn=admins,ou=groups,dc=example,dc=org": "admins",
		"jdoe":                                  "jdoe",
		"":                                      "unknown",
	}
	for dn, want := range cases {
		if got := rdnValue(dn); got != want {
			t.Errorf("rdnValue(%q) = %q, want %q", dn, got, want)
		}
	}
}

// TestResourceURL checks the details link uses the same base64url DN encoding
// as the frontend's encodeDN (btoa + URL-safe replacement, padding stripped).
func TestResourceURL(t *testing.T) {
	m := NewMailer(&config.MailConfig{}, "Example", "https://ldap.example.org/")
	dn := "uid=jdoe,ou=people,dc=example,dc=org"
	got := m.resourceURL("user", dn)

	const prefix = "https://ldap.example.org/users/"
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("resourceURL = %q, want prefix %q", got, prefix)
	}
	enc := strings.TrimPrefix(got, prefix)
	decoded, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("encoded segment is not valid base64url: %v", err)
	}
	if string(decoded) != dn {
		t.Errorf("decoded DN = %q, want %q", string(decoded), dn)
	}

	if url := m.resourceURL("sudorole", dn); url != "" {
		t.Errorf("sudorole has no web page, want empty URL, got %q", url)
	}
}

// TestRenderAuditTemplate verifies the diff rows render and that the <style>
// block is inlined into style="" attributes (premailer), since several mail
// clients drop <head> styles.
func TestRenderAuditTemplate(t *testing.T) {
	m := NewMailer(&config.MailConfig{}, "Example", "https://ldap.example.org")
	data := auditEmailData{
		Organization: "Example",
		Subject:      "Modification of Romain Besson",
		Actor:        "prodige",
		HasChanges:   true,
		Changes: []auditChangeView{
			{Field: "Email", Old: "old@x.org", New: "new@x.org"},
			{Field: "Password", Masked: true},
		},
		DetailsURL: "https://ldap.example.org/users/abc",
		Timestamp:  "2026-06-09 16:29:31 UTC",
	}
	body, err := m.renderTemplate(auditNotificationTemplate, data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"Modification of Romain Besson", "new@x.org", "old@x.org", "View details", "updated"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q\n%s", want, body)
		}
	}
	// Masked password value must never leak — only the badge text.
	if strings.Contains(body, "Password</td>") && strings.Contains(body, "•") {
		t.Errorf("masked field leaked a value")
	}
	// Premailer should have produced inline style attributes.
	if !strings.Contains(body, "style=") {
		t.Errorf("CSS was not inlined; no style attributes found\n%s", body)
	}
}

// TestRenderAuditFieldList verifies creations render a "field: value" dump
// (no strike-through "old → new" arrows) and that the masked field shows a
// "set" badge rather than any value.
func TestRenderAuditFieldList(t *testing.T) {
	m := NewMailer(&config.MailConfig{}, "Example", "")
	data := auditEmailData{
		Organization: "Example",
		Subject:      "New account created: Romain Besson",
		Actor:        "prodige",
		HasChanges:   true,
		IsFieldList:  true,
		Changes: []auditChangeView{
			{Field: "Email", New: "romain.besson@nievre.fr"},
			{Field: "Password", Masked: true},
		},
		Summary:   "A new account was created: Romain Besson.",
		Timestamp: "2026-06-09 16:29:31 UTC",
	}
	body, err := m.renderTemplate(auditNotificationTemplate, data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(body, "romain.besson@nievre.fr") {
		t.Errorf("field value missing")
	}
	if !strings.Contains(body, ">set<") {
		t.Errorf("masked field should show a 'set' badge")
	}
	if strings.Contains(body, "&rarr;") || strings.Contains(body, "→") {
		t.Errorf("field list must not render diff arrows")
	}
}
