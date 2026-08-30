package ldap

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type Group struct {
	DN          string   `json:"dn"`
	CN          string   `json:"cn"`
	GIDNumber   int      `json:"gidNumber"`
	Description string   `json:"description,omitempty"`
	MemberUIDs  []string `json:"memberUid,omitempty"`
	// Samba attributes (sambaGroupMapping)
	SambaSID       string   `json:"sambaSID,omitempty"`
	SambaGroupType string   `json:"sambaGroupType,omitempty"`
	DisplayName    string   `json:"displayName,omitempty"`
	ObjectClasses  []string `json:"objectClasses"`
}

type CreateGroupRequest struct {
	CN          string   `json:"cn"`
	GIDNumber   int      `json:"gidNumber"`
	Description string   `json:"description,omitempty"`
	MemberUIDs  []string `json:"memberUid,omitempty"`
}

type UpdateGroupRequest struct {
	Description *string  `json:"description,omitempty"`
	MemberUIDs  []string `json:"memberUid,omitempty"`
}

var defaultGroupAttributes = []string{
	"dn", "cn", "gidNumber", "description", "memberUid", "objectClass",
	// Samba attributes
	"sambaSID", "sambaGroupType", "displayName",
}

// GroupListItem is what the group list endpoint returns: a row's worth of
// fields plus the member count, instead of every group's full member list.
type GroupListItem struct {
	DN           string `json:"dn"`
	CN           string `json:"cn"`
	GIDNumber    int    `json:"gidNumber"`
	Description  string `json:"description,omitempty"`
	MembersCount int    `json:"membersCount"`
}

// listGroupAttributes still has to read memberUid — LDAP cannot count values
// server-side — but only the count crosses the wire to the browser.
var listGroupAttributes = []string{"dn", "cn", "gidNumber", "description", "memberUid"}

// GroupPage is one page of a group listing.
type GroupPage struct {
	Items     []GroupListItem
	Total     int
	Truncated bool
}

// SearchGroupsPage lists groups matching an optional substring search, sorted
// and sliced to one page.
func (c *Client) SearchGroupsPage(opts ListOptions) (*GroupPage, error) {
	filter := andFilter("(objectClass=posixGroup)", substringFilter(opts.Search, "cn", "description"))
	entries, truncated, err := c.SearchLimited(c.GroupBaseDN(), filter, listGroupAttributes)
	if err != nil {
		return nil, err
	}

	items := make([]GroupListItem, 0, len(entries))
	for _, entry := range entries {
		gid, _ := strconv.Atoi(entry.GetAttributeValue("gidNumber"))
		items = append(items, GroupListItem{
			DN:           entry.DN,
			CN:           entry.GetAttributeValue("cn"),
			GIDNumber:    gid,
			Description:  entry.GetAttributeValue("description"),
			MembersCount: len(entry.GetAttributeValues("memberUid")),
		})
	}

	switch opts.Sort {
	case "description":
		sortBy(items, opts.Desc, func(a, b GroupListItem) bool { return foldLess(a.Description, b.Description) })
	case "gidNumber":
		sortBy(items, opts.Desc, func(a, b GroupListItem) bool { return a.GIDNumber < b.GIDNumber })
	case "members":
		sortBy(items, opts.Desc, func(a, b GroupListItem) bool { return a.MembersCount < b.MembersCount })
	default:
		sortBy(items, opts.Desc, func(a, b GroupListItem) bool { return foldLess(a.CN, b.CN) })
	}

	return &GroupPage{
		Items:     page(items, opts),
		Total:     len(items),
		Truncated: truncated,
	}, nil
}

// memberGroupCounts maps each uid to the number of groups it belongs to. Used
// to fill the user list's Groups column from one group search.
func (c *Client) memberGroupCounts() (map[string]int, error) {
	entries, _, err := c.SearchLimited(c.GroupBaseDN(), "(objectClass=posixGroup)", []string{"memberUid"})
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, entry := range entries {
		for _, uid := range entry.GetAttributeValues("memberUid") {
			counts[uid]++
		}
	}
	return counts, nil
}

func (c *Client) GetGroup(dn string) (*Group, error) {
	entry, err := c.GetEntry(dn, defaultGroupAttributes)
	if err != nil {
		return nil, err
	}

	group := entryToGroup(entry)
	return &group, nil
}

func (c *Client) GetGroupByCN(cn string) (*Group, error) {
	entries, err := c.Search(c.GroupBaseDN(), fmt.Sprintf("(cn=%s)", ldap.EscapeFilter(cn)), defaultGroupAttributes)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("group not found: %s", cn)
	}

	group := entryToGroup(entries[0])
	return &group, nil
}

func (c *Client) CreateGroup(req CreateGroupRequest) (*Group, error) {
	// See reserveUIDNumber: the same check, for the same reason. Two groups on
	// one gidNumber are one group to the kernel.
	gidNumber, err := c.reserveGIDNumber(req.GIDNumber)
	if err != nil {
		return nil, err
	}
	req.GIDNumber = gidNumber

	dn := fmt.Sprintf("cn=%s,%s", ldap.EscapeDN(req.CN), c.GroupBaseDN())

	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", []string{"posixGroup"})
	addReq.Attribute("cn", []string{req.CN})
	addReq.Attribute("gidNumber", []string{strconv.Itoa(req.GIDNumber)})

	if req.Description != "" {
		addReq.Attribute("description", []string{req.Description})
	}

	if len(req.MemberUIDs) > 0 {
		addReq.Attribute("memberUid", req.MemberUIDs)
	}

	if err := c.Add(addReq); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}

	return c.GetGroup(dn)
}

func (c *Client) UpdateGroup(dn string, req UpdateGroupRequest) (*Group, error) {
	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	// Replace, never Delete: see clearing. Clearing a description a group never
	// had, or emptying a membership that was already empty, asked the directory
	// to remove an absent attribute and came back as an internal server error.
	if req.Description != nil {
		modReq.Replace("description", clearing(*req.Description))
		hasChanges = true
	}

	if req.MemberUIDs != nil {
		modReq.Replace("memberUid", clearing(req.MemberUIDs...))
		hasChanges = true
	}

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update group: %w", err)
		}
	}

	return c.GetGroup(dn)
}

func (c *Client) DeleteGroup(dn string) error {
	return c.Delete(dn)
}

func (c *Client) AddGroupMember(groupDN, memberUID string) error {
	group, err := c.GetGroup(groupDN)
	if err != nil {
		return err
	}

	for _, uid := range group.MemberUIDs {
		if uid == memberUID {
			return nil
		}
	}

	modReq := ldap.NewModifyRequest(groupDN, nil)
	modReq.Add("memberUid", []string{memberUID})

	return c.Modify(modReq)
}

func (c *Client) RemoveGroupMember(groupDN, memberUID string) error {
	modReq := ldap.NewModifyRequest(groupDN, nil)
	modReq.Delete("memberUid", []string{memberUID})

	return c.Modify(modReq)
}

// GroupMember is one entry of a group's membership, resolved to something worth
// displaying.
//
// A group stores memberUid values, not references, so the uid is the only thing
// the group entry itself can tell you. The rest comes from the user entries.
type GroupMember struct {
	// UID is always set; it is the value stored on the group.
	UID string `json:"uid"`
	// The remaining fields are empty when no user entry carries this uid — a
	// dangling membership, which the UI shows as the bare uid.
	DN          string `json:"dn,omitempty"`
	CN          string `json:"cn,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	HasPhoto    bool   `json:"hasPhoto"`
	// Whether this member can actually use what the group grants. A membership
	// list is where dead weight accumulates — an account locked by an
	// administrator months ago still appears to hold every access its groups
	// confer — so the row says so rather than looking like any other.
	PwdAccountLockedTime string   `json:"pwdAccountLockedTime,omitempty"`
	PwdEndTime           string   `json:"pwdEndTime,omitempty"`
	Lockout              *Lockout `json:"lockout,omitempty"`
}

// GroupMembers resolves a group's memberUid values to user entries.
//
// One search with an OR filter over the group's uids, rather than the browser
// downloading every user to look names up: a group has as many members as it has
// members, while the directory has as many users as it has users. Members with
// no matching user entry are still returned, uid only, because a dangling
// membership is something the operator needs to see rather than a row that
// silently disappears.
func (c *Client) GroupMembers(groupDN string) ([]GroupMember, error) {
	group, err := c.GetGroup(groupDN)
	if err != nil {
		return nil, err
	}
	if len(group.MemberUIDs) == 0 {
		return []GroupMember{}, nil
	}

	var uids strings.Builder
	uids.WriteString("(|")
	for _, uid := range group.MemberUIDs {
		fmt.Fprintf(&uids, "(uid=%s)", ldap.EscapeFilter(uid))
	}
	uids.WriteString(")")
	memberFilter := "(&(objectClass=inetOrgPerson)" + uids.String() + ")"

	entries, err := c.Search(c.UserBaseDN(), memberFilter,
		[]string{"uid", "cn", "displayName", lockAttr, expiryAttr, "pwdPolicySubentry"})
	if err != nil {
		return nil, err
	}

	// Which of them have a photo, asked for as dn only: the answer is a boolean
	// per row, and requesting jpegPhoto would put every member's photo in this
	// payload just to compute it. Same trick as usersWithPhoto.
	withPhoto := make(map[string]bool)
	if photos, perr := c.Search(c.UserBaseDN(),
		"(&(objectClass=inetOrgPerson)(jpegPhoto=*)"+uids.String()+")", []string{"dn"}); perr == nil {
		for _, entry := range photos {
			withPhoto[entry.DN] = true
		}
	}

	// One resolver for the whole group: the policies it reads are the same handful
	// for every member, as on the users list.
	lockouts := c.newLockoutResolver()

	resolved := make(map[string]GroupMember, len(entries))
	for _, entry := range entries {
		uid := entry.GetAttributeValue("uid")
		lockedAt := entry.GetAttributeValue(lockAttr)
		resolved[uid] = GroupMember{
			UID:                  uid,
			DN:                   entry.DN,
			CN:                   entry.GetAttributeValue("cn"),
			DisplayName:          entry.GetAttributeValue("displayName"),
			HasPhoto:             withPhoto[entry.DN],
			PwdAccountLockedTime: lockedAt,
			PwdEndTime:           entry.GetAttributeValue(expiryAttr),
			Lockout:              lockouts.resolve(lockedAt, entry.GetAttributeValue("pwdPolicySubentry")),
		}
	}

	members := make([]GroupMember, 0, len(group.MemberUIDs))
	for _, uid := range group.MemberUIDs {
		if member, ok := resolved[uid]; ok {
			members = append(members, member)
			continue
		}
		members = append(members, GroupMember{UID: uid})
	}
	sort.Slice(members, func(i, j int) bool {
		return strings.ToLower(members[i].UID) < strings.ToLower(members[j].UID)
	})

	return members, nil
}

func (c *Client) GetUserGroups(uid string) ([]Group, error) {
	entries, err := c.Search(c.GroupBaseDN(), fmt.Sprintf("(memberUid=%s)", ldap.EscapeFilter(uid)), defaultGroupAttributes)
	if err != nil {
		return nil, err
	}

	groups := make([]Group, 0, len(entries))
	for _, entry := range entries {
		groups = append(groups, entryToGroup(entry))
	}

	return groups, nil
}

func entryToGroup(entry *ldap.Entry) Group {
	gidNumber, _ := strconv.Atoi(entry.GetAttributeValue("gidNumber"))

	return Group{
		DN:          entry.DN,
		CN:          entry.GetAttributeValue("cn"),
		GIDNumber:   gidNumber,
		Description: entry.GetAttributeValue("description"),
		MemberUIDs:  entry.GetAttributeValues("memberUid"),
		// Samba attributes
		SambaSID:       entry.GetAttributeValue("sambaSID"),
		SambaGroupType: entry.GetAttributeValue("sambaGroupType"),
		DisplayName:    entry.GetAttributeValue("displayName"),
		ObjectClasses:  entry.GetAttributeValues("objectClass"),
	}
}

// UpdateSambaGroupRequest contains Samba-specific group attributes
type UpdateSambaGroupRequest struct {
	SambaSID       *string `json:"sambaSID,omitempty"`
	SambaGroupType *string `json:"sambaGroupType,omitempty"`
	DisplayName    *string `json:"displayName,omitempty"`
}

// SetSambaGroupAttributes updates Samba attributes for a group
func (c *Client) SetSambaGroupAttributes(dn string, req UpdateSambaGroupRequest) (*Group, error) {
	group, err := c.GetGroup(dn)
	if err != nil {
		return nil, err
	}

	// Check if group has sambaGroupMapping objectClass
	hasObjectClass := false
	for _, oc := range group.ObjectClasses {
		if oc == "sambaGroupMapping" {
			hasObjectClass = true
			break
		}
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	// Add objectClass if not present and we have a SID to set
	if !hasObjectClass && req.SambaSID != nil && *req.SambaSID != "" {
		modReq.Add("objectClass", []string{"sambaGroupMapping"})
		hasChanges = true
	}

	// Helper to check if an attribute has a value
	hasAttribute := func(attr string) bool {
		switch attr {
		case "sambaSID":
			return group.SambaSID != ""
		case "sambaGroupType":
			return group.SambaGroupType != ""
		case "displayName":
			return group.DisplayName != ""
		}
		return false
	}

	addModify := func(attr string, value *string) {
		if value != nil {
			if *value == "" {
				if hasAttribute(attr) {
					modReq.Delete(attr, nil)
					hasChanges = true
				}
			} else {
				modReq.Replace(attr, []string{*value})
				hasChanges = true
			}
		}
	}

	addModify("sambaSID", req.SambaSID)
	addModify("sambaGroupType", req.SambaGroupType)
	addModify("displayName", req.DisplayName)

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update samba attributes: %w", err)
		}
	}

	return c.GetGroup(dn)
}

// RenameGroup changes a group's cn, which is its RDN, and migrates the
// references the directory will not.
//
// Far less invasive than renaming a user, and for one reason: a POSIX group is
// joined by number, not by name. Its members sit in memberUid as plain uids and
// its primary members carry gidNumber, so neither notices the rename. What does
// notice is sudo, which names a group as "%cn" in sudoUser — a bare string the
// directory does not track, exactly like "+name" for a netgroup. Left alone,
// those rules would go on granting to a name that no longer exists, silently.
//
// Returns the new DN and the references it could not migrate. A failure to
// rewrite one is reported, never fatal: the entry has already been renamed by
// then, and unwinding that would leave the directory in a third state nobody
// asked for.
func (c *Client) RenameGroup(oldDN, oldCN, newCN string) (string, []string, error) {
	if existing, err := c.GetGroupByCN(newCN); err == nil && existing != nil {
		return "", nil, fmt.Errorf("a group named %q already exists", newCN)
	}

	newDN := fmt.Sprintf("cn=%s,%s", ldap.EscapeDN(newCN), c.GroupBaseDN())

	// deleteOldRDN drops the old cn value, so the cn attribute matches the new
	// RDN rather than keeping both.
	if err := c.ModifyDN(oldDN, "cn="+ldap.EscapeDN(newCN), true); err != nil {
		return "", nil, fmt.Errorf("rename entry: %w", err)
	}

	// Non-nil so it serialises as an empty JSON array, not null.
	warnings := []string{}
	warn := func(what string, err error) {
		warnings = append(warnings, fmt.Sprintf("%s: %v", what, err))
	}

	// Sudo roles granting to "%oldCN". Only exact matches are swapped, so a uid
	// or an "ALL" that happens to look similar is left alone.
	if roles, err := c.GetGroupSudoRoles(oldCN); err != nil {
		warn("read sudo roles", err)
	} else {
		for _, role := range roles {
			users := replaceExact(role.SudoUser, "%"+oldCN, "%"+newCN)
			if _, err := c.UpdateSudoRole(role.DN, UpdateSudoRoleRequest{SudoUser: users}); err != nil {
				warn(fmt.Sprintf("update sudo role %q", role.CN), err)
			}
		}
	}

	// sudoRunAsGroup names a group to run as, without the "%" prefix — so the
	// value is indistinguishable from a group that only exists on the hosts.
	// Rewriting it would be a guess about which one was meant, so it is reported
	// instead, the same way a rename reports what it could not reach.
	if roles, err := c.sudoRolesRunningAsGroup(oldCN); err != nil {
		warn("check sudoRunAsGroup references", err)
	} else {
		for _, role := range roles {
			warnings = append(warnings, fmt.Sprintf(
				"sudo role %q runs as group %q, which no longer exists here; "+
					"check whether it meant this group or one local to the hosts",
				role.CN, oldCN))
		}
	}

	return newDN, warnings, nil
}
