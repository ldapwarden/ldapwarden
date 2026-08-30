package ldap

import "testing"

// TestAlignSambaSID pins which derivations are recognised and, more importantly,
// which are refused. A SID is the identity Windows uses; rewriting one whose RID
// came from somewhere else would swap the account for a different principal.
func TestAlignSambaSID(t *testing.T) {
	cases := []struct {
		name   string
		sid    string
		want   string
		wantOK bool
	}{
		{
			// What this app writes when it offers a default.
			name:   "rid is the uid number",
			sid:    "S-1-5-21-0-0-0-1200",
			want:   "S-1-5-21-0-0-0-1500",
			wantOK: true,
		},
		{
			// Samba's conventional algorithm: 2*1200+1000 = 3400.
			name:   "rid is 2*uid+1000",
			sid:    "S-1-5-21-111-222-333-3400",
			want:   "S-1-5-21-111-222-333-4000",
			wantOK: true,
		},
		{
			// A real domain SID whose RID was allocated elsewhere. Leaving it wrong
			// is recoverable; rewriting it to a RID that belongs to someone else is
			// not.
			name: "rid unrelated to the uid number",
			sid:  "S-1-5-21-111-222-333-513",
		},
		{name: "not a sid", sid: "nonsense"},
		{name: "rid not a number", sid: "S-1-5-21-0-0-0-abc"},
		{name: "empty", sid: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := alignSambaSID(tc.sid, 1200, 1500)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.wantOK, got)
			}
			if ok && got != tc.want {
				t.Errorf("alignSambaSID(%q) = %q, want %q", tc.sid, got, tc.want)
			}
		})
	}

	// The domain part identifies the domain, not the account, and must survive.
	if got, _ := alignSambaSID("S-1-5-21-111-222-333-1200", 1200, 1500); got != "S-1-5-21-111-222-333-1500" {
		t.Errorf("the domain part was altered: %q", got)
	}
}
