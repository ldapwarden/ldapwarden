//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
)

type pagedListResponse struct {
	Data []struct {
		DN        string `json:"dn"`
		CN        string `json:"cn"`
		SudoOrder int    `json:"sudoOrder"`
	} `json:"data"`
	Total     int  `json:"total"`
	Limit     int  `json:"limit"`
	Offset    int  `json:"offset"`
	Truncated bool `json:"truncated"`
}

func listPaged(t *testing.T, env *testEnv, token, path, query string) pagedListResponse {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, path+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s%s: status=%d body=%s", path, query, resp.StatusCode, body)
	}
	var out pagedListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal %s: %v (body=%s)", path, err, body)
	}
	return out
}

// TestIntegration_SudoRoles_Paging checks the sudo-role listing pages and sorts
// over the whole match, the way the users and groups listings do — the total has
// to describe the set, not the slice.
func TestIntegration_SudoRoles_Paging(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	all := listPaged(t, env, token, "/api/sudo-roles/", "")
	if all.Total < 2 {
		t.Skipf("need at least 2 sudo roles in the directory, got %d", all.Total)
	}

	first := listPaged(t, env, token, "/api/sudo-roles/", "?limit=1&sort=cn")
	if len(first.Data) != 1 {
		t.Fatalf("limit=1 returned %d entries", len(first.Data))
	}
	if first.Total != all.Total {
		t.Errorf("total = %d, want the whole match %d", first.Total, all.Total)
	}

	second := listPaged(t, env, token, "/api/sudo-roles/", "?limit=1&offset=1&sort=cn")
	if len(second.Data) != 1 {
		t.Fatalf("offset=1 returned %d entries", len(second.Data))
	}
	if first.Data[0].CN == second.Data[0].CN {
		t.Errorf("offset had no effect: both pages start at %q", first.Data[0].CN)
	}

	// Descending page 1 must hold the overall last entry, which is the whole
	// point of sorting before slicing.
	asc := listPaged(t, env, token, "/api/sudo-roles/", "?sort=cn&order=asc")
	names := make([]string, len(asc.Data))
	for i, r := range asc.Data {
		names[i] = strings.ToLower(r.CN)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("ascending sort is not ordered: %v", names)
	}
	desc := listPaged(t, env, token, "/api/sudo-roles/", "?sort=cn&order=desc&limit=1")
	if want := asc.Data[len(asc.Data)-1].CN; desc.Data[0].CN != want {
		t.Errorf("first row of the descending page = %q, want the overall last %q", desc.Data[0].CN, want)
	}
}

// TestIntegration_PasswordPolicies_Paging is the same contract for policies.
func TestIntegration_PasswordPolicies_Paging(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	all := listPaged(t, env, token, "/api/password-policies/", "")
	if all.Total < 2 {
		t.Skipf("need at least 2 password policies in the directory, got %d", all.Total)
	}

	page := listPaged(t, env, token, "/api/password-policies/", "?limit=1&sort=cn")
	if len(page.Data) != 1 {
		t.Fatalf("limit=1 returned %d entries", len(page.Data))
	}
	if page.Total != all.Total {
		t.Errorf("total = %d, want the whole match %d", page.Total, all.Total)
	}

	beyond := listPaged(t, env, token, "/api/password-policies/", "?limit=5&offset=999")
	if len(beyond.Data) != 0 {
		t.Errorf("page past the end returned %d entries", len(beyond.Data))
	}
	if beyond.Total != all.Total {
		t.Errorf("total on an out-of-range page = %d, want %d", beyond.Total, all.Total)
	}
}

// TestIntegration_SudoRoles_SearchStillMatchesMembers guards the search fields:
// a sudo role is findable by the user it grants to, not only by its name.
func TestIntegration_SudoRoles_SearchStillMatchesMembers(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	all := listPaged(t, env, token, "/api/sudo-roles/", "")
	var probe string
	for _, role := range all.Data {
		resp, body := doJSON(t, env, http.MethodGet, "/api/sudo-roles/"+url.QueryEscape(role.DN), nil, token)
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var full struct {
			SudoUser []string `json:"sudoUser"`
		}
		if err := json.Unmarshal(body, &full); err == nil && len(full.SudoUser) > 0 {
			probe = full.SudoUser[0]
			break
		}
	}
	if probe == "" || strings.HasPrefix(probe, "%") || probe == "ALL" {
		t.Skip("no sudo role with a plain sudoUser to search for")
	}

	found := listPaged(t, env, token, "/api/sudo-roles/", "?search="+url.QueryEscape(probe))
	if found.Total == 0 {
		t.Errorf("searching for sudoUser %q matched nothing", probe)
	}
}
