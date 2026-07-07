package mail

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// TestDumpPreview is a throwaway helper: when LDAPWARDEN_MAIL_PREVIEW is set it
// renders a few audit scenarios to that HTML path so the result can be eyeballed
// in a browser. It is skipped during normal test runs.
func TestDumpPreview(t *testing.T) {
	out := os.Getenv("LDAPWARDEN_MAIL_PREVIEW")
	if out == "" {
		t.Skip("set LDAPWARDEN_MAIL_PREVIEW=/path/to/out.html to render a preview")
	}
	m := NewMailer(&config.MailConfig{}, "Anyware Services", "https://hive.anyware.corp")
	ts := time.Date(2026, 6, 9, 16, 29, 31, 0, time.UTC)

	scenarios := []struct {
		action, rType, dn, name string
		changes                 []map[string]string
		details                 map[string]interface{}
	}{
		{
			action: "user.update", rType: "user",
			dn: "uid=aduffez,ou=people,dc=anyware,dc=corp", name: "Alexandre Duffez (aduffez)",
			details: map[string]interface{}{"action": "password_reset_sent"},
		},
		{
			action: "user.update", rType: "user",
			dn: "uid=rbesson,ou=people,dc=anyware,dc=corp", name: "Romain Besson (rbesson)",
			changes: []map[string]string{
				{"field": "Display name", "old": "Romain B.", "new": "Romain Besson"},
				{"field": "Email", "old": "rbesson@old.fr", "new": "romain.besson@nievre.fr"},
				{"field": "Account active", "old": "True", "new": "False"},
				{"field": "Password", "masked": "true"},
			},
		},
		{
			action: "user.create", rType: "user",
			dn: "uid=rbesson,ou=people,dc=anyware,dc=corp", name: "Romain Besson (rbesson)",
			changes: []map[string]string{
				{"field": "Username", "new": "rbesson"},
				{"field": "Email", "new": "romain.besson@nievre.fr"},
				{"field": "Organization", "new": "CD58"},
				{"field": "Expiration date", "new": "2027-06-09"},
				{"field": "Groups", "new": "external, anyware-ext"},
				{"field": "Password", "masked": "true"},
			},
		},
		{
			action: "group.member.add", rType: "group",
			dn: "cn=anyware-ext,ou=groups,dc=anyware,dc=corp", name: "External staff (anyware-ext)",
			details: map[string]interface{}{"memberUid": "rbesson"},
		},
	}

	var html string
	for _, s := range scenarios {
		actor := "Lionel Porcheron (lionel)"
		typeLabel := humanResourceType(s.rType)
		views := toChangeViews(s.changes)
		data := auditEmailData{
			Organization: m.organization,
			Subject:      auditSubject(s.action, typeLabel, s.name),
			Actor:        actor,
			HasChanges:   len(views) > 0,
			IsFieldList:  strings.HasSuffix(s.action, ".create"),
			Changes:      views,
			Summary:      auditSummary(s.action, typeLabel, s.name, s.details),
			DetailsURL:   m.resourceURL(s.rType, s.dn),
			Timestamp:    ts.Format("2006-01-02 15:04:05 MST"),
			ActorDN:      "uid=prodige,ou=people,dc=anyware,dc=corp",
			ResourceDN:   s.dn,
			IPAddress:    "10.0.0.42",
		}
		body, err := m.renderTemplate(auditNotificationTemplate, data)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		html += "<hr style='margin:32px 0'>" + body
	}
	if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("preview written to %s", out)
}
