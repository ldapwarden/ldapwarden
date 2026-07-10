package api

import "testing"

func TestValidateSudoCN(t *testing.T) {
	valid := []string{"webadmins", "%wheel", "%infra", "role_1", "a.b-c", "%"}
	for _, v := range valid {
		if err := validateSudoCN(v); err != nil {
			t.Errorf("validateSudoCN(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{"", "with space", "comma,injection", "plus+val", "quote\"", "back\\slash"}
	for _, v := range invalid {
		if err := validateSudoCN(v); err == nil {
			t.Errorf("validateSudoCN(%q) = nil, want error", v)
		}
	}
}

// The shared RDN validator must stay strict: '%' is allowed only for sudo CNs,
// not for user uids or group cns.
func TestValidateRDNValueRejectsPercent(t *testing.T) {
	if err := validateRDNValue("uid", "%wheel"); err == nil {
		t.Errorf("validateRDNValue should reject '%%' for a uid/cn")
	}
}
