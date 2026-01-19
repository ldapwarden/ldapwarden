package ldap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type SudoRole struct {
	DN             string   `json:"dn"`
	CN             string   `json:"cn"`
	Description    string   `json:"description,omitempty"`
	SudoUser       []string `json:"sudoUser,omitempty"`
	SudoHost       []string `json:"sudoHost,omitempty"`
	SudoCommand    []string `json:"sudoCommand,omitempty"`
	SudoRunAs      []string `json:"sudoRunAs,omitempty"`
	SudoRunAsUser  []string `json:"sudoRunAsUser,omitempty"`
	SudoRunAsGroup []string `json:"sudoRunAsGroup,omitempty"`
	SudoOption     []string `json:"sudoOption,omitempty"`
	SudoOrder      int      `json:"sudoOrder,omitempty"`
	SudoNotBefore  string   `json:"sudoNotBefore,omitempty"`
	SudoNotAfter   string   `json:"sudoNotAfter,omitempty"`
}

type CreateSudoRoleRequest struct {
	CN             string   `json:"cn"`
	Description    string   `json:"description,omitempty"`
	SudoUser       []string `json:"sudoUser"`
	SudoHost       []string `json:"sudoHost"`
	SudoCommand    []string `json:"sudoCommand"`
	SudoRunAs      []string `json:"sudoRunAs,omitempty"`
	SudoRunAsUser  []string `json:"sudoRunAsUser,omitempty"`
	SudoRunAsGroup []string `json:"sudoRunAsGroup,omitempty"`
	SudoOption     []string `json:"sudoOption,omitempty"`
	SudoOrder      int      `json:"sudoOrder,omitempty"`
	SudoNotBefore  string   `json:"sudoNotBefore,omitempty"`
	SudoNotAfter   string   `json:"sudoNotAfter,omitempty"`
}

type UpdateSudoRoleRequest struct {
	Description    *string  `json:"description,omitempty"`
	SudoUser       []string `json:"sudoUser,omitempty"`
	SudoHost       []string `json:"sudoHost,omitempty"`
	SudoCommand    []string `json:"sudoCommand,omitempty"`
	SudoRunAs      []string `json:"sudoRunAs,omitempty"`
	SudoRunAsUser  []string `json:"sudoRunAsUser,omitempty"`
	SudoRunAsGroup []string `json:"sudoRunAsGroup,omitempty"`
	SudoOption     []string `json:"sudoOption,omitempty"`
	SudoOrder      *int     `json:"sudoOrder,omitempty"`
	SudoNotBefore  *string  `json:"sudoNotBefore,omitempty"`
	SudoNotAfter   *string  `json:"sudoNotAfter,omitempty"`
}

var defaultSudoAttributes = []string{
	"dn", "cn", "description", "sudoUser", "sudoHost", "sudoCommand",
	"sudoRunAs", "sudoRunAsUser", "sudoRunAsGroup", "sudoOption",
	"sudoOrder", "sudoNotBefore", "sudoNotAfter",
}

func (c *Client) SudoBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.SudoersOU, c.config.BaseDN)
}

// ListSudoRoles returns all sudo roles
func (c *Client) ListSudoRoles() ([]SudoRole, error) {
	entries, err := c.Search(c.SudoBaseDN(), "(objectClass=sudoRole)", defaultSudoAttributes)
	if err != nil {
		return nil, err
	}

	roles := make([]SudoRole, 0, len(entries))
	for _, entry := range entries {
		roles = append(roles, entryToSudoRole(entry))
	}

	return roles, nil
}

// GetSudoRole returns a sudo role by DN
func (c *Client) GetSudoRole(dn string) (*SudoRole, error) {
	entry, err := c.GetEntry(dn, defaultSudoAttributes)
	if err != nil {
		return nil, err
	}

	role := entryToSudoRole(entry)
	return &role, nil
}

// GetUserSudoRoles returns all sudo roles that apply to a specific user
func (c *Client) GetUserSudoRoles(uid string) ([]SudoRole, error) {
	// Build filter parts: direct user match, ALL, and group-based matches
	filterParts := []string{
		fmt.Sprintf("(sudoUser=%s)", ldap.EscapeFilter(uid)),
		"(sudoUser=ALL)",
	}

	// Get groups the user belongs to and add %groupname filters
	groups, err := c.GetUserGroups(uid)
	if err == nil {
		for _, group := range groups {
			// Sudo uses %groupname format for group-based access
			filterParts = append(filterParts, fmt.Sprintf("(sudoUser=%%%s)", ldap.EscapeFilter(group.CN)))
		}
	}

	// Build the complete filter
	filter := fmt.Sprintf("(&(objectClass=sudoRole)(|%s))", strings.Join(filterParts, ""))
	entries, err := c.Search(c.SudoBaseDN(), filter, defaultSudoAttributes)
	if err != nil {
		return nil, err
	}

	roles := make([]SudoRole, 0, len(entries))
	for _, entry := range entries {
		roles = append(roles, entryToSudoRole(entry))
	}

	return roles, nil
}

// CreateSudoRole creates a new sudo role
func (c *Client) CreateSudoRole(req CreateSudoRoleRequest) (*SudoRole, error) {
	dn := fmt.Sprintf("cn=%s,%s", req.CN, c.SudoBaseDN())

	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", []string{"sudoRole"})
	addReq.Attribute("cn", []string{req.CN})

	if req.Description != "" {
		addReq.Attribute("description", []string{req.Description})
	}
	if len(req.SudoUser) > 0 {
		addReq.Attribute("sudoUser", req.SudoUser)
	}
	if len(req.SudoHost) > 0 {
		addReq.Attribute("sudoHost", req.SudoHost)
	}
	if len(req.SudoCommand) > 0 {
		addReq.Attribute("sudoCommand", req.SudoCommand)
	}
	if len(req.SudoRunAs) > 0 {
		addReq.Attribute("sudoRunAs", req.SudoRunAs)
	}
	if len(req.SudoRunAsUser) > 0 {
		addReq.Attribute("sudoRunAsUser", req.SudoRunAsUser)
	}
	if len(req.SudoRunAsGroup) > 0 {
		addReq.Attribute("sudoRunAsGroup", req.SudoRunAsGroup)
	}
	if len(req.SudoOption) > 0 {
		addReq.Attribute("sudoOption", req.SudoOption)
	}
	if req.SudoOrder != 0 {
		addReq.Attribute("sudoOrder", []string{strconv.Itoa(req.SudoOrder)})
	}
	if req.SudoNotBefore != "" {
		addReq.Attribute("sudoNotBefore", []string{req.SudoNotBefore})
	}
	if req.SudoNotAfter != "" {
		addReq.Attribute("sudoNotAfter", []string{req.SudoNotAfter})
	}

	if err := c.Add(addReq); err != nil {
		return nil, fmt.Errorf("create sudo role: %w", err)
	}

	return c.GetSudoRole(dn)
}

// UpdateSudoRole updates an existing sudo role
func (c *Client) UpdateSudoRole(dn string, req UpdateSudoRoleRequest) (*SudoRole, error) {
	current, err := c.GetSudoRole(dn)
	if err != nil {
		return nil, err
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	updateMultiValue := func(attr string, newVals []string, currentVals []string) {
		if newVals != nil {
			if len(newVals) > 0 {
				if len(currentVals) > 0 {
					modReq.Replace(attr, newVals)
				} else {
					modReq.Add(attr, newVals)
				}
			} else if len(currentVals) > 0 {
				modReq.Delete(attr, nil)
			}
			hasChanges = true
		}
	}

	updateSingleValue := func(attr string, newVal *string, currentVal string) {
		if newVal != nil {
			if *newVal != "" {
				if currentVal != "" {
					modReq.Replace(attr, []string{*newVal})
				} else {
					modReq.Add(attr, []string{*newVal})
				}
			} else if currentVal != "" {
				modReq.Delete(attr, nil)
			}
			hasChanges = true
		}
	}

	updateSingleValue("description", req.Description, current.Description)
	updateMultiValue("sudoUser", req.SudoUser, current.SudoUser)
	updateMultiValue("sudoHost", req.SudoHost, current.SudoHost)
	updateMultiValue("sudoCommand", req.SudoCommand, current.SudoCommand)
	updateMultiValue("sudoRunAs", req.SudoRunAs, current.SudoRunAs)
	updateMultiValue("sudoRunAsUser", req.SudoRunAsUser, current.SudoRunAsUser)
	updateMultiValue("sudoRunAsGroup", req.SudoRunAsGroup, current.SudoRunAsGroup)
	updateMultiValue("sudoOption", req.SudoOption, current.SudoOption)
	updateSingleValue("sudoNotBefore", req.SudoNotBefore, current.SudoNotBefore)
	updateSingleValue("sudoNotAfter", req.SudoNotAfter, current.SudoNotAfter)

	if req.SudoOrder != nil {
		if *req.SudoOrder != 0 {
			if current.SudoOrder != 0 {
				modReq.Replace("sudoOrder", []string{strconv.Itoa(*req.SudoOrder)})
			} else {
				modReq.Add("sudoOrder", []string{strconv.Itoa(*req.SudoOrder)})
			}
		} else if current.SudoOrder != 0 {
			modReq.Delete("sudoOrder", nil)
		}
		hasChanges = true
	}

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update sudo role: %w", err)
		}
	}

	return c.GetSudoRole(dn)
}

// DeleteSudoRole deletes a sudo role
func (c *Client) DeleteSudoRole(dn string) error {
	return c.Delete(dn)
}

// AddUserToSudoRole adds a user to an existing sudo role
func (c *Client) AddUserToSudoRole(sudoRoleDN, uid string) error {
	modReq := ldap.NewModifyRequest(sudoRoleDN, nil)
	modReq.Add("sudoUser", []string{uid})
	return c.Modify(modReq)
}

// RemoveUserFromSudoRole removes a user from a sudo role
func (c *Client) RemoveUserFromSudoRole(sudoRoleDN, uid string) error {
	modReq := ldap.NewModifyRequest(sudoRoleDN, nil)
	modReq.Delete("sudoUser", []string{uid})
	return c.Modify(modReq)
}

// GetGroupSudoRoles returns all sudo roles that apply to a specific group
func (c *Client) GetGroupSudoRoles(groupCN string) ([]SudoRole, error) {
	// Groups in sudoUser are prefixed with %
	filter := fmt.Sprintf("(&(objectClass=sudoRole)(sudoUser=%%%s))", ldap.EscapeFilter(groupCN))
	entries, err := c.Search(c.SudoBaseDN(), filter, defaultSudoAttributes)
	if err != nil {
		return nil, err
	}

	roles := make([]SudoRole, 0, len(entries))
	for _, entry := range entries {
		roles = append(roles, entryToSudoRole(entry))
	}

	return roles, nil
}

// AddGroupToSudoRole adds a group to an existing sudo role (with % prefix)
func (c *Client) AddGroupToSudoRole(sudoRoleDN, groupCN string) error {
	modReq := ldap.NewModifyRequest(sudoRoleDN, nil)
	modReq.Add("sudoUser", []string{"%" + groupCN})
	return c.Modify(modReq)
}

// RemoveGroupFromSudoRole removes a group from a sudo role
func (c *Client) RemoveGroupFromSudoRole(sudoRoleDN, groupCN string) error {
	modReq := ldap.NewModifyRequest(sudoRoleDN, nil)
	modReq.Delete("sudoUser", []string{"%" + groupCN})
	return c.Modify(modReq)
}

func entryToSudoRole(entry *ldap.Entry) SudoRole {
	sudoOrder, _ := strconv.Atoi(entry.GetAttributeValue("sudoOrder"))

	return SudoRole{
		DN:             entry.DN,
		CN:             entry.GetAttributeValue("cn"),
		Description:    entry.GetAttributeValue("description"),
		SudoUser:       entry.GetAttributeValues("sudoUser"),
		SudoHost:       entry.GetAttributeValues("sudoHost"),
		SudoCommand:    entry.GetAttributeValues("sudoCommand"),
		SudoRunAs:      entry.GetAttributeValues("sudoRunAs"),
		SudoRunAsUser:  entry.GetAttributeValues("sudoRunAsUser"),
		SudoRunAsGroup: entry.GetAttributeValues("sudoRunAsGroup"),
		SudoOption:     entry.GetAttributeValues("sudoOption"),
		SudoOrder:      sudoOrder,
		SudoNotBefore:  entry.GetAttributeValue("sudoNotBefore"),
		SudoNotAfter:   entry.GetAttributeValue("sudoNotAfter"),
	}
}
