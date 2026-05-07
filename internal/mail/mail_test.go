package mail

import (
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
	m := NewMailer(&config.MailConfig{From: "noreply@example.org"}, "Example")

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
	m := NewMailer(&config.MailConfig{From: "noreply@example.org"}, "Example")
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
