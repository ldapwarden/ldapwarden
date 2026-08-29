//go:build integration

package ldap

import (
	"errors"
	"strings"
	"testing"
)

// TestIntegration_MissingBase covers the failure an existing directory is most
// likely to produce: the app is pointed at a tree that has no ou=netgroups, or
// no ou=sudoers, because nothing ever needed one there.
//
// It used to surface as "internal server error", which names the wrong culprit —
// the fix is an ldapadd or one line of configuration, not a bug report. The
// error is typed so the API can say which setting to look at.
func TestIntegration_MissingBase(t *testing.T) {
	client := netgroupTestClient(t)

	cfg := client.config
	cfg.NetgroupOU = "ou=ldapwarden-missing-on-purpose"
	missingClient := NewClient(cfg)
	defer missingClient.Close()

	_, err := missingClient.SearchNetgroupsPage(ListOptions{})
	if err == nil {
		t.Fatal("searching a base the directory does not hold succeeded")
	}

	missing, ok := AsMissingBase(err)
	if !ok {
		t.Fatalf("error is not a MissingBaseError: %v", err)
	}
	if missing.Setting != "LDAP_NETGROUP_OU" {
		t.Errorf("Setting = %q, want the setting an operator would change", missing.Setting)
	}
	if missing.Collection != "netgroups" {
		t.Errorf("Collection = %q, want %q", missing.Collection, "netgroups")
	}
	// The underlying result code stays reachable, so nothing downstream that
	// inspects it is cut off by the wrapping.
	if !strings.Contains(err.Error(), "No Such Object") {
		t.Errorf("the original directory error was lost: %v", err)
	}
	if !errors.Is(err, missing.Unwrap()) {
		t.Error("Unwrap does not lead back to the directory error")
	}

	// A create has the same problem from the other side: the parent is missing,
	// not the entry. It must not report an internal error either.
	_, err = missingClient.CreateNetgroup(CreateNetgroupRequest{
		CN:      "ldapwarden-should-not-be-created",
		Triples: []NetgroupTriple{{User: "jdoe"}},
	})
	if err == nil {
		t.Fatal("creating an entry under a base that does not exist succeeded")
	}
	if _, ok := AsMissingBase(err); !ok {
		t.Errorf("a create under a missing base is not reported as such: %v", err)
	}
}

// TestIntegration_MissingBaseDNs covers the startup check, which is where an
// operator can still act before a user runs into it.
func TestIntegration_MissingBaseDNs(t *testing.T) {
	client := netgroupTestClient(t)

	if missing := client.MissingBaseDNs(); len(missing) != 0 {
		t.Errorf("the bundled directory reports missing bases: %+v", missing)
	}

	cfg := client.config
	cfg.NetgroupOU = "ou=ldapwarden-missing-on-purpose"
	cfg.SudoersOU = "ou=ldapwarden-also-missing"
	broken := NewClient(cfg)
	defer broken.Close()

	missing := broken.MissingBaseDNs()
	if len(missing) != 2 {
		t.Fatalf("reported %d missing bases, want the two that are: %+v", len(missing), missing)
	}
	settings := map[string]bool{}
	for _, base := range missing {
		settings[base.Setting] = true
		if base.DN == "" || base.Collection == "" {
			t.Errorf("a missing base is reported without enough to act on: %+v", base)
		}
	}
	for _, want := range []string{"LDAP_NETGROUP_OU", "LDAP_SUDOERS_OU"} {
		if !settings[want] {
			t.Errorf("%s is not among the reported settings: %+v", want, missing)
		}
	}
}

// A missing *entry* is not a missing base. Conflating them would turn every
// ordinary 404 into "your configuration is wrong".
func TestIntegration_MissingEntryIsNotAMissingBase(t *testing.T) {
	client := netgroupTestClient(t)

	_, err := client.GetNetgroup("cn=ldapwarden-no-such-netgroup," + client.NetgroupBaseDN())
	if err == nil {
		t.Fatal("reading a netgroup that does not exist succeeded")
	}
	if missing, ok := AsMissingBase(err); ok {
		t.Errorf("a missing entry was reported as a missing base (%+v)", missing)
	}
}
