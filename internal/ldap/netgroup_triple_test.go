package ldap

import "testing"

func TestParseNetgroupTriple(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    NetgroupTriple
		wantErr bool
	}{
		{
			name: "all three slots",
			raw:  "(web01,jdoe,example.org)",
			want: NetgroupTriple{Host: "web01", User: "jdoe", Domain: "example.org"},
		},
		{
			name: "user netgroup: empty host means any host",
			raw:  "(,jdoe,)",
			want: NetgroupTriple{User: "jdoe"},
		},
		{
			name: "host netgroup: empty user means any user",
			raw:  "(web01,,)",
			want: NetgroupTriple{Host: "web01"},
		},
		{
			name: "everything open",
			raw:  "(,,)",
			want: NetgroupTriple{},
		},
		{
			// Tolerated for portability, not because OpenLDAP produces it: it
			// validates the syntax and rejects "( a , b , c )" outright. A server
			// that does not validate can still hand us one.
			name: "whitespace around the value and inside the slots",
			raw:  "  ( web01 , jdoe , example.org )  ",
			want: NetgroupTriple{Host: "web01", User: "jdoe", Domain: "example.org"},
		},
		{
			// "-" matches nothing, where "" matches everything. Losing the
			// distinction widens a grant, so it has to survive parsing intact.
			name: "the NIS no-match marker is preserved, not treated as empty",
			raw:  "(-,jdoe,)",
			want: NetgroupTriple{Host: NetgroupWildcard, User: "jdoe"},
		},
		{
			name:    "no parentheses",
			raw:     "web01,jdoe,example.org",
			wantErr: true,
		},
		{
			name:    "only an opening parenthesis",
			raw:     "(web01,jdoe,example.org",
			wantErr: true,
		},
		{
			name:    "two fields",
			raw:     "(web01,jdoe)",
			wantErr: true,
		},
		{
			name:    "four fields",
			raw:     "(web01,jdoe,example.org,extra)",
			wantErr: true,
		},
		{
			name:    "empty value",
			raw:     "",
			wantErr: true,
		},
		{
			// A netgroup name on its own belongs in memberNisNetgroup. Seeing one
			// here means the entry was written by something that confused the two,
			// and guessing would hide that.
			name:    "a bare netgroup name is not a triple",
			raw:     "ops-junior",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseNetgroupTriple(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseNetgroupTriple(%q) = %+v, want an error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseNetgroupTriple(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ParseNetgroupTriple(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNetgroupTripleString(t *testing.T) {
	cases := []struct {
		name   string
		triple NetgroupTriple
		want   string
	}{
		{"all three", NetgroupTriple{Host: "web01", User: "jdoe", Domain: "example.org"}, "(web01,jdoe,example.org)"},
		{"user only", NetgroupTriple{User: "jdoe"}, "(,jdoe,)"},
		{"host only", NetgroupTriple{Host: "web01"}, "(web01,,)"},
		{"empty", NetgroupTriple{}, "(,,)"},
		{"no-match marker", NetgroupTriple{Host: NetgroupWildcard, User: "jdoe"}, "(-,jdoe,)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.triple.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNetgroupTripleRoundTrip is the property the rewrite paths depend on:
// parsing then formatting must not change what the triple means. Spacing is
// normalised, which is why the canonical spelling is compared rather than the
// input.
func TestNetgroupTripleRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"(web01,jdoe,example.org)",
		"(,jdoe,)",
		"(web01,,)",
		"(,,)",
		"(-,jdoe,)",
		"(web01,-,example.org)",
	} {
		parsed, err := ParseNetgroupTriple(raw)
		if err != nil {
			t.Fatalf("ParseNetgroupTriple(%q): %v", raw, err)
		}
		if got := parsed.String(); got != raw {
			t.Errorf("round trip of %q produced %q", raw, got)
		}
		again, err := ParseNetgroupTriple(parsed.String())
		if err != nil {
			t.Fatalf("re-parse of %q: %v", parsed.String(), err)
		}
		if again != parsed {
			t.Errorf("re-parse of %q gave %+v, want %+v", raw, again, parsed)
		}
	}
}

// TestNetgroupTripleNamesUser guards the distinction the rename path turns on: a
// slot that names the user, versus one left open, versus one excluding it.
func TestNetgroupTripleNamesUser(t *testing.T) {
	cases := []struct {
		name   string
		triple NetgroupTriple
		uid    string
		want   bool
	}{
		{"names the user", NetgroupTriple{User: "jdoe"}, "jdoe", true},
		{"open slot is not a name", NetgroupTriple{}, "jdoe", false},
		{"no-match marker is not a name", NetgroupTriple{User: NetgroupWildcard}, "jdoe", false},
		{"a different user", NetgroupTriple{User: "alice"}, "jdoe", false},
		{
			// The bug a substring rewrite would introduce.
			name:   "a user whose name contains the uid",
			triple: NetgroupTriple{User: "jdoe-backup"},
			uid:    "jdoe",
			want:   false,
		},
		{
			// And the mirror: the uid sitting in the host slot must not count.
			name:   "the uid appears in the host slot",
			triple: NetgroupTriple{Host: "jdoe", User: "alice"},
			uid:    "jdoe",
			want:   false,
		},
		{"empty uid never matches", NetgroupTriple{}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.triple.NamesUser(tc.uid); got != tc.want {
				t.Errorf("NamesUser(%q) on %+v = %v, want %v", tc.uid, tc.triple, got, tc.want)
			}
		})
	}
}
