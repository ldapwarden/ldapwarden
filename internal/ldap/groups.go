package ldap

import (
	"fmt"
	"strconv"

	"github.com/go-ldap/ldap/v3"
)

type Group struct {
	DN          string   `json:"dn"`
	CN          string   `json:"cn"`
	GIDNumber   int      `json:"gidNumber"`
	Description string   `json:"description,omitempty"`
	MemberUIDs  []string `json:"memberUid,omitempty"`
	// Samba attributes (sambaGroupMapping)
	SambaSID       string `json:"sambaSID,omitempty"`
	SambaGroupType string `json:"sambaGroupType,omitempty"`
	DisplayName    string `json:"displayName,omitempty"`
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

func (c *Client) ListGroups() ([]Group, error) {
	entries, err := c.Search(c.GroupBaseDN(), "(objectClass=posixGroup)", defaultGroupAttributes)
	if err != nil {
		return nil, err
	}

	groups := make([]Group, 0, len(entries))
	for _, entry := range entries {
		groups = append(groups, entryToGroup(entry))
	}

	return groups, nil
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

	if req.Description != nil {
		if *req.Description == "" {
			// LDAP doesn't allow empty values, delete the attribute instead
			modReq.Delete("description", nil)
		} else {
			modReq.Replace("description", []string{*req.Description})
		}
		hasChanges = true
	}

	if req.MemberUIDs != nil {
		if len(req.MemberUIDs) > 0 {
			modReq.Replace("memberUid", req.MemberUIDs)
		} else {
			modReq.Delete("memberUid", nil)
		}
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
