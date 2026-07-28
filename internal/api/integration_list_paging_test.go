//go:build integration

package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// userListResponse mirrors the shape of GET /api/users.
type userListResponse struct {
	Data          []ldap.UserListItem `json:"data"`
	Total         int                 `json:"total"`
	InactiveCount int                 `json:"inactiveCount"`
	Limit         int                 `json:"limit"`
	Offset        int                 `json:"offset"`
	Truncated     bool                `json:"truncated"`
}

type groupListResponse struct {
	Data      []ldap.GroupListItem `json:"data"`
	Total     int                  `json:"total"`
	Truncated bool                 `json:"truncated"`
}

func listUsers(t *testing.T, env *testEnv, token, query string) userListResponse {
	t.Helper()
	resp, body := doJSON(t, env, http.MethodGet, "/api/users/"+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users %q: status=%d body=%s", query, resp.StatusCode, body)
	}
	var out userListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal user list: %v (body=%s)", err, body)
	}
	return out
}

// TestIntegration_ListUsers_PagingIsServerSide checks that limit/offset slice
// the result set and that Total keeps describing the whole match, not the page.
func TestIntegration_ListUsers_PagingIsServerSide(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	all := listUsers(t, env, token, "")
	if all.Total < 3 {
		t.Skipf("need at least 3 users in the directory, got %d", all.Total)
	}

	first := listUsers(t, env, token, "?limit=2&sort=uid")
	if len(first.Data) != 2 {
		t.Fatalf("limit=2 returned %d entries", len(first.Data))
	}
	if first.Total != all.Total {
		t.Errorf("total should count every match, not the page: got %d want %d", first.Total, all.Total)
	}
	if first.Limit != 2 || first.Offset != 0 {
		t.Errorf("echoed paging = limit %d offset %d, want 2/0", first.Limit, first.Offset)
	}

	second := listUsers(t, env, token, "?limit=2&offset=2&sort=uid")
	if len(second.Data) == 0 {
		t.Fatal("offset=2 returned an empty page")
	}
	if first.Data[0].UID == second.Data[0].UID {
		t.Errorf("offset had no effect: both pages start at %q", first.Data[0].UID)
	}

	// A page past the end is empty rather than an error, so a stale page number
	// in a bookmarked URL cannot break the view.
	beyond := listUsers(t, env, token, fmt.Sprintf("?limit=10&offset=%d", all.Total+100))
	if len(beyond.Data) != 0 {
		t.Errorf("page past the end returned %d entries", len(beyond.Data))
	}
	if beyond.Total != all.Total {
		t.Errorf("total on an out-of-range page = %d, want %d", beyond.Total, all.Total)
	}
}

// TestIntegration_ListUsers_SortIsServerSide checks ordering is applied across
// the whole result set, which is the point of sorting server-side: page 1 of a
// descending sort must hold the last entries overall.
func TestIntegration_ListUsers_SortIsServerSide(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	asc := listUsers(t, env, token, "?sort=uid&order=asc")
	if len(asc.Data) < 2 {
		t.Skip("need at least 2 users")
	}
	uids := make([]string, len(asc.Data))
	for i, u := range asc.Data {
		uids[i] = strings.ToLower(u.UID)
	}
	if !sort.StringsAreSorted(uids) {
		t.Errorf("ascending sort is not ordered: %v", uids)
	}

	desc := listUsers(t, env, token, "?sort=uid&order=desc&limit=1")
	if len(desc.Data) != 1 {
		t.Fatalf("limit=1 returned %d entries", len(desc.Data))
	}
	if want := asc.Data[len(asc.Data)-1].UID; desc.Data[0].UID != want {
		t.Errorf("first row of the descending page = %q, want the overall last uid %q", desc.Data[0].UID, want)
	}
}

// TestIntegration_ListUsers_StatusFilter checks a locked account leaves the
// active listing while still being counted, which is what labels the
// "Show inactive (N)" toggle.
func TestIntegration_ListUsers_StatusFilter(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	search := "?search=" + url.QueryEscape(user.UID)

	before := listUsers(t, env, token, search+"&status=active")
	if len(before.Data) != 1 {
		t.Fatalf("new user not in the active listing: %d entries", len(before.Data))
	}

	if resp, body := doJSON(t, env, http.MethodPost, userPath(user.DN)+"/lock", nil, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("lock user: status=%d body=%s", resp.StatusCode, body)
	}

	active := listUsers(t, env, token, search+"&status=active")
	if len(active.Data) != 0 {
		t.Errorf("locked user still in the active listing: %+v", active.Data)
	}
	if active.InactiveCount < 1 {
		t.Errorf("inactiveCount = %d, want at least 1 — the toggle label depends on it", active.InactiveCount)
	}

	inactive := listUsers(t, env, token, search+"&status=inactive")
	if len(inactive.Data) != 1 {
		t.Fatalf("locked user missing from the inactive listing: %d entries", len(inactive.Data))
	}
	if !inactive.Data[0].AccountLocked {
		t.Error("accountLocked not reported on the list projection")
	}

	unfiltered := listUsers(t, env, token, search)
	if len(unfiltered.Data) != 1 {
		t.Errorf("unfiltered listing = %d entries, want 1", len(unfiltered.Data))
	}
}

// TestIntegration_ListUsers_ProjectionExcludesPhoto pins the payload contract:
// the list carries a hasPhoto flag, never the photo itself.
func TestIntegration_ListUsers_ProjectionExcludesPhoto(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)
	photo := []byte{0xff, 0xd8, 0xff, 0xd9} // shortest thing that is recognisably a JPEG
	if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
		map[string]any{"jpegPhoto": base64.StdEncoding.EncodeToString(photo)}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("set photo: status=%d body=%s", resp.StatusCode, body)
	}

	query := "?search=" + url.QueryEscape(user.UID)
	resp, body := doJSON(t, env, http.MethodGet, "/api/users/"+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users: status=%d body=%s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "jpegPhoto") {
		t.Errorf("list response leaks the photo payload: %s", body)
	}

	var out userListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("expected the created user, got %d entries", len(out.Data))
	}
	if !out.Data[0].HasPhoto {
		t.Error("hasPhoto = false after setting a photo")
	}
}

// TestIntegration_UserPhotoEndpoint checks the photo is served as a cacheable
// image, and that a user without one yields 404 (so the avatar falls back to
// initials instead of rendering a broken image).
func TestIntegration_UserPhotoEndpoint(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	user := createTestUser(t, env, token)

	resp, _ := doJSON(t, env, http.MethodGet, userPath(user.DN)+"/photo", nil, token)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("photo of a user without one: status=%d, want 404", resp.StatusCode)
	}

	photo := []byte{0xff, 0xd8, 0xff, 0xd9}
	if resp, body := doJSON(t, env, http.MethodPut, userPath(user.DN),
		map[string]any{"jpegPhoto": base64.StdEncoding.EncodeToString(photo)}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("set photo: status=%d body=%s", resp.StatusCode, body)
	}

	resp, body := doJSON(t, env, http.MethodGet, userPath(user.DN)+"/photo", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get photo: status=%d body=%s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "private") {
		t.Errorf("Cache-Control = %q, want it to mark the response private", cc)
	}
	if string(body) != string(photo) {
		t.Errorf("photo body = %x, want %x", body, photo)
	}
}

// TestIntegration_ListGroups_ProjectionCountsMembers checks the group listing
// reports a member count instead of shipping every member uid.
func TestIntegration_ListGroups_ProjectionCountsMembers(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	group := createTestGroup(t, env, token)
	user := createTestUser(t, env, token)
	if resp, body := doJSON(t, env, http.MethodPost, groupPath(group.DN)+"/members",
		map[string]string{"memberUid": user.UID}, token); resp.StatusCode != http.StatusOK {
		t.Fatalf("add member: status=%d body=%s", resp.StatusCode, body)
	}

	query := "?search=" + url.QueryEscape(group.CN)
	resp, body := doJSON(t, env, http.MethodGet, "/api/groups/"+query, nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list groups: status=%d body=%s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "memberUid") {
		t.Errorf("group list leaks the member list: %s", body)
	}

	var out groupListResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("expected the created group, got %d entries", len(out.Data))
	}
	if out.Data[0].MembersCount != 1 {
		t.Errorf("membersCount = %d, want 1", out.Data[0].MembersCount)
	}

	// The user list's Groups column is filled from the same directory data.
	users := listUsers(t, env, token, "?search="+url.QueryEscape(user.UID))
	if len(users.Data) != 1 {
		t.Fatalf("expected the created user, got %d entries", len(users.Data))
	}
	if users.Data[0].GroupsCount != 1 {
		t.Errorf("groupsCount = %d, want 1", users.Data[0].GroupsCount)
	}
}

// TestIntegration_ListUsers_LimitIsCapped checks an over-large limit cannot turn
// the paged endpoint back into an unbounded dump.
func TestIntegration_ListUsers_LimitIsCapped(t *testing.T) {
	env := setupTestServer(t)
	token := loginAs(t, env, "admin", "admin123").Token

	out := listUsers(t, env, token, "?limit=999999")
	if out.Limit != maxListLimit {
		t.Errorf("limit = %d, want it capped at %d", out.Limit, maxListLimit)
	}

	// A malformed limit falls back to "no paging" rather than failing the call.
	if resp, _ := doJSON(t, env, http.MethodGet, "/api/users/?limit=abc", nil, token); resp.StatusCode != http.StatusOK {
		t.Errorf("malformed limit: status=%d, want 200", resp.StatusCode)
	}
}
