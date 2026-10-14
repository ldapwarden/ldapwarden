package ldap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestAttrValueEncoding(t *testing.T) {
	cases := []struct {
		name  string
		value []byte
		want  string
	}{
		{"plain", []byte("Jane Doe"), "cn: Jane Doe"},
		{"empty", []byte{}, "cn:"},
		{"non-ascii", []byte("Zoë"), "cn:: Wm/Dqw=="},
		{"leading space", []byte(" x"), "cn:: IHg="},
		{"leading colon", []byte(":x"), "cn:: Ong="},
		{"leading less-than", []byte("<x"), "cn:: PHg="},
		{"trailing space", []byte("x "), "cn:: eCA="},
		{"newline inside", []byte("a\nb"), "cn:: YQpi"},
		{"binary", []byte{0xff, 0xd8, 0x00}, "cn:: /9gA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := attrValue("cn", tc.value); got != tc.want {
				t.Errorf("attrValue(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestLDIFWriterFoldsLongLines(t *testing.T) {
	var buf bytes.Buffer
	w := newLDIFWriter(&buf)
	w.line("description: " + strings.Repeat("x", 200))
	if err := w.flush(); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	for i, line := range lines {
		if len(line) > ldifLineWidth+1 {
			t.Errorf("line %d is %d bytes long", i, len(line))
		}
		if i > 0 && !strings.HasPrefix(line, " ") {
			t.Errorf("continuation line %d does not start with a space: %q", i, line)
		}
	}
	// Unfolding — dropping each newline and the space after it — gives the value back.
	if unfolded := strings.ReplaceAll(buf.String(), "\n ", ""); unfolded != "description: "+strings.Repeat("x", 200)+"\n" {
		t.Errorf("unfolded output differs: %q", unfolded)
	}
}

// TestParentFirstHoldsBackEarlyChildren: search order is creation order on
// back-mdb, so an entry moved under a newer OU arrives before it. ldapadd
// refuses an entry whose parent does not exist yet.
func TestParentFirstHoldsBackEarlyChildren(t *testing.T) {
	var order []string
	p := newParentFirst(true, "dc=example,dc=org", func(e *ldap.Entry) { order = append(order, e.DN) })

	for _, dn := range []string{
		"dc=example,dc=org",
		"uid=moved,ou=New,dc=example,dc=org",         // before its parent
		"cn=deep,uid=moved,ou=New,dc=example,dc=org", // before its grandparent
		"ou=People,dc=example,dc=org",
		"OU=new, DC=Example,DC=org", // the parent, spelled differently
	} {
		p.add(&ldap.Entry{DN: dn})
	}
	if orphans := p.flushOrphans(); orphans != 0 {
		t.Errorf("orphans = %d, want 0", orphans)
	}

	want := []string{
		"dc=example,dc=org",
		"ou=People,dc=example,dc=org",
		"OU=new, DC=Example,DC=org",
		"uid=moved,ou=New,dc=example,dc=org",
		"cn=deep,uid=moved,ou=New,dc=example,dc=org",
	}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Errorf("order:\n got %v\nwant %v", order, want)
	}
}

func TestParentFirstWritesOrphansLast(t *testing.T) {
	var order []string
	p := newParentFirst(true, "dc=example,dc=org", func(e *ldap.Entry) { order = append(order, e.DN) })
	p.add(&ldap.Entry{DN: "dc=example,dc=org"})
	p.add(&ldap.Entry{DN: "uid=x,ou=Hidden,dc=example,dc=org"})

	if orphans := p.flushOrphans(); orphans != 1 {
		t.Errorf("orphans = %d, want 1", orphans)
	}
	if len(order) != 2 || order[1] != "uid=x,ou=Hidden,dc=example,dc=org" {
		t.Errorf("the orphan was not written: %v", order)
	}
	if stats := p.stats(); stats.Entries != 2 || stats.Orphans != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestParseExportScope(t *testing.T) {
	if scope, err := ParseExportScope(""); err != nil || scope != ExportSubtree {
		t.Errorf("default scope = %q, %v", scope, err)
	}
	if _, err := ParseExportScope("children"); err == nil {
		t.Error("unknown scope accepted")
	}
}
