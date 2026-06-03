package ldap

import (
	"fmt"
	"strconv"

	"github.com/go-ldap/ldap/v3"
)

// PasswordPolicy represents an LDAP password policy (pwdPolicy objectClass)
type PasswordPolicy struct {
	DN                      string `json:"dn"`
	CN                      string `json:"cn"`
	Description             string `json:"description,omitempty"`
	PwdAttribute            string `json:"pwdAttribute,omitempty"`
	PwdMinAge               int    `json:"pwdMinAge,omitempty"`
	PwdMaxAge               int    `json:"pwdMaxAge,omitempty"`
	PwdInHistory            int    `json:"pwdInHistory,omitempty"`
	PwdCheckQuality         int    `json:"pwdCheckQuality,omitempty"`
	PwdMinLength            int    `json:"pwdMinLength,omitempty"`
	PwdExpireWarning        int    `json:"pwdExpireWarning,omitempty"`
	PwdGraceAuthNLimit      int    `json:"pwdGraceAuthNLimit,omitempty"`
	PwdLockout              bool   `json:"pwdLockout"`
	PwdLockoutDuration      int    `json:"pwdLockoutDuration,omitempty"`
	PwdMaxFailure           int    `json:"pwdMaxFailure,omitempty"`
	PwdFailureCountInterval int    `json:"pwdFailureCountInterval,omitempty"`
	PwdMustChange           bool   `json:"pwdMustChange"`
	PwdAllowUserChange      bool   `json:"pwdAllowUserChange"`
	PwdSafeModify           bool   `json:"pwdSafeModify"`
	PwdCheckModule          string `json:"pwdCheckModule,omitempty"`
}

// CreatePasswordPolicyRequest is used for creating new password policies
type CreatePasswordPolicyRequest struct {
	CN                      string `json:"cn"`
	Description             string `json:"description,omitempty"`
	PwdAttribute            string `json:"pwdAttribute"`
	PwdMinAge               int    `json:"pwdMinAge,omitempty"`
	PwdMaxAge               int    `json:"pwdMaxAge,omitempty"`
	PwdInHistory            int    `json:"pwdInHistory,omitempty"`
	PwdCheckQuality         int    `json:"pwdCheckQuality,omitempty"`
	PwdMinLength            int    `json:"pwdMinLength,omitempty"`
	PwdExpireWarning        int    `json:"pwdExpireWarning,omitempty"`
	PwdGraceAuthNLimit      int    `json:"pwdGraceAuthNLimit,omitempty"`
	PwdLockout              bool   `json:"pwdLockout"`
	PwdLockoutDuration      int    `json:"pwdLockoutDuration,omitempty"`
	PwdMaxFailure           int    `json:"pwdMaxFailure,omitempty"`
	PwdFailureCountInterval int    `json:"pwdFailureCountInterval,omitempty"`
	PwdMustChange           bool   `json:"pwdMustChange"`
	PwdAllowUserChange      bool   `json:"pwdAllowUserChange"`
	PwdSafeModify           bool   `json:"pwdSafeModify"`
	PwdCheckModule          string `json:"pwdCheckModule,omitempty"`
}

// UpdatePasswordPolicyRequest is used for updating password policies
type UpdatePasswordPolicyRequest struct {
	Description             *string `json:"description,omitempty"`
	PwdAttribute            *string `json:"pwdAttribute,omitempty"`
	PwdMinAge               *int    `json:"pwdMinAge,omitempty"`
	PwdMaxAge               *int    `json:"pwdMaxAge,omitempty"`
	PwdInHistory            *int    `json:"pwdInHistory,omitempty"`
	PwdCheckQuality         *int    `json:"pwdCheckQuality,omitempty"`
	PwdMinLength            *int    `json:"pwdMinLength,omitempty"`
	PwdExpireWarning        *int    `json:"pwdExpireWarning,omitempty"`
	PwdGraceAuthNLimit      *int    `json:"pwdGraceAuthNLimit,omitempty"`
	PwdLockout              *bool   `json:"pwdLockout,omitempty"`
	PwdLockoutDuration      *int    `json:"pwdLockoutDuration,omitempty"`
	PwdMaxFailure           *int    `json:"pwdMaxFailure,omitempty"`
	PwdFailureCountInterval *int    `json:"pwdFailureCountInterval,omitempty"`
	PwdMustChange           *bool   `json:"pwdMustChange,omitempty"`
	PwdAllowUserChange      *bool   `json:"pwdAllowUserChange,omitempty"`
	PwdSafeModify           *bool   `json:"pwdSafeModify,omitempty"`
	PwdCheckModule          *string `json:"pwdCheckModule,omitempty"`
}

var defaultPolicyAttributes = []string{
	"dn", "cn", "description",
	"pwdAttribute", "pwdMinAge", "pwdMaxAge", "pwdInHistory",
	"pwdCheckQuality", "pwdMinLength", "pwdExpireWarning",
	"pwdGraceAuthNLimit", "pwdLockout", "pwdLockoutDuration",
	"pwdMaxFailure", "pwdFailureCountInterval", "pwdMustChange",
	"pwdAllowUserChange", "pwdSafeModify", "pwdCheckModule",
}

func (c *Client) PpolicyBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.PpolicyOU, c.config.BaseDN)
}

// ListPasswordPolicies returns all password policies
func (c *Client) ListPasswordPolicies() ([]PasswordPolicy, error) {
	entries, err := c.Search(c.PpolicyBaseDN(), "(objectClass=pwdPolicy)", defaultPolicyAttributes)
	if err != nil {
		return nil, err
	}

	policies := make([]PasswordPolicy, 0, len(entries))
	for _, entry := range entries {
		policies = append(policies, entryToPasswordPolicy(entry))
	}

	return policies, nil
}

// SearchPasswordPolicies lists policies matching an optional substring search
// (cn, description), bounded by the configured search size limit. It returns
// truncated=true when more matches exist than were returned.
func (c *Client) SearchPasswordPolicies(search string) ([]PasswordPolicy, bool, error) {
	filter := andFilter("(objectClass=pwdPolicy)", substringFilter(search, "cn", "description"))
	entries, truncated, err := c.SearchLimited(c.PpolicyBaseDN(), filter, defaultPolicyAttributes)
	if err != nil {
		return nil, false, err
	}

	policies := make([]PasswordPolicy, 0, len(entries))
	for _, entry := range entries {
		policies = append(policies, entryToPasswordPolicy(entry))
	}

	return policies, truncated, nil
}

// GetPasswordPolicy returns a password policy by DN
func (c *Client) GetPasswordPolicy(dn string) (*PasswordPolicy, error) {
	entry, err := c.GetEntry(dn, defaultPolicyAttributes)
	if err != nil {
		return nil, err
	}

	policy := entryToPasswordPolicy(entry)
	return &policy, nil
}

// CreatePasswordPolicy creates a new password policy
func (c *Client) CreatePasswordPolicy(req CreatePasswordPolicyRequest) (*PasswordPolicy, error) {
	dn := fmt.Sprintf("cn=%s,%s", ldap.EscapeDN(req.CN), c.PpolicyBaseDN())

	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", []string{"pwdPolicy", "person", "top"})
	addReq.Attribute("cn", []string{req.CN})
	addReq.Attribute("sn", []string{req.CN}) // sn is required for person

	if req.Description != "" {
		addReq.Attribute("description", []string{req.Description})
	}
	if req.PwdAttribute != "" {
		addReq.Attribute("pwdAttribute", []string{req.PwdAttribute})
	} else {
		// Use OID for userPassword (2.5.4.35) as required by ppolicy schema
		addReq.Attribute("pwdAttribute", []string{"2.5.4.35"})
	}
	if req.PwdMinAge > 0 {
		addReq.Attribute("pwdMinAge", []string{strconv.Itoa(req.PwdMinAge)})
	}
	if req.PwdMaxAge > 0 {
		addReq.Attribute("pwdMaxAge", []string{strconv.Itoa(req.PwdMaxAge)})
	}
	if req.PwdInHistory > 0 {
		addReq.Attribute("pwdInHistory", []string{strconv.Itoa(req.PwdInHistory)})
	}
	if req.PwdCheckQuality > 0 {
		addReq.Attribute("pwdCheckQuality", []string{strconv.Itoa(req.PwdCheckQuality)})
	}
	if req.PwdMinLength > 0 {
		addReq.Attribute("pwdMinLength", []string{strconv.Itoa(req.PwdMinLength)})
	}
	if req.PwdExpireWarning > 0 {
		addReq.Attribute("pwdExpireWarning", []string{strconv.Itoa(req.PwdExpireWarning)})
	}
	if req.PwdGraceAuthNLimit > 0 {
		addReq.Attribute("pwdGraceAuthNLimit", []string{strconv.Itoa(req.PwdGraceAuthNLimit)})
	}
	if req.PwdLockout {
		addReq.Attribute("pwdLockout", []string{"TRUE"})
	}
	if req.PwdLockoutDuration > 0 {
		addReq.Attribute("pwdLockoutDuration", []string{strconv.Itoa(req.PwdLockoutDuration)})
	}
	if req.PwdMaxFailure > 0 {
		addReq.Attribute("pwdMaxFailure", []string{strconv.Itoa(req.PwdMaxFailure)})
	}
	if req.PwdFailureCountInterval > 0 {
		addReq.Attribute("pwdFailureCountInterval", []string{strconv.Itoa(req.PwdFailureCountInterval)})
	}
	if req.PwdMustChange {
		addReq.Attribute("pwdMustChange", []string{"TRUE"})
	}
	if req.PwdAllowUserChange {
		addReq.Attribute("pwdAllowUserChange", []string{"TRUE"})
	} else {
		addReq.Attribute("pwdAllowUserChange", []string{"FALSE"})
	}
	if req.PwdSafeModify {
		addReq.Attribute("pwdSafeModify", []string{"TRUE"})
	}
	if req.PwdCheckModule != "" {
		addReq.Attribute("pwdCheckModule", []string{req.PwdCheckModule})
	}

	if err := c.Add(addReq); err != nil {
		return nil, fmt.Errorf("create password policy: %w", err)
	}

	return c.GetPasswordPolicy(dn)
}

// UpdatePasswordPolicy updates an existing password policy
func (c *Client) UpdatePasswordPolicy(dn string, req UpdatePasswordPolicyRequest) (*PasswordPolicy, error) {
	current, err := c.GetPasswordPolicy(dn)
	if err != nil {
		return nil, err
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	updateStringValue := func(attr string, newVal *string, currentVal string) {
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

	updateIntValue := func(attr string, newVal *int, currentVal int) {
		if newVal != nil {
			if *newVal > 0 {
				if currentVal > 0 {
					modReq.Replace(attr, []string{strconv.Itoa(*newVal)})
				} else {
					modReq.Add(attr, []string{strconv.Itoa(*newVal)})
				}
			} else if currentVal > 0 {
				modReq.Delete(attr, nil)
			}
			hasChanges = true
		}
	}

	updateBoolValue := func(attr string, newVal *bool, currentVal bool) {
		if newVal != nil {
			val := "FALSE"
			if *newVal {
				val = "TRUE"
			}
			modReq.Replace(attr, []string{val})
			hasChanges = true
		}
	}

	updateStringValue("description", req.Description, current.Description)
	updateStringValue("pwdAttribute", req.PwdAttribute, current.PwdAttribute)
	updateIntValue("pwdMinAge", req.PwdMinAge, current.PwdMinAge)
	updateIntValue("pwdMaxAge", req.PwdMaxAge, current.PwdMaxAge)
	updateIntValue("pwdInHistory", req.PwdInHistory, current.PwdInHistory)
	updateIntValue("pwdCheckQuality", req.PwdCheckQuality, current.PwdCheckQuality)
	updateIntValue("pwdMinLength", req.PwdMinLength, current.PwdMinLength)
	updateIntValue("pwdExpireWarning", req.PwdExpireWarning, current.PwdExpireWarning)
	updateIntValue("pwdGraceAuthNLimit", req.PwdGraceAuthNLimit, current.PwdGraceAuthNLimit)
	updateBoolValue("pwdLockout", req.PwdLockout, current.PwdLockout)
	updateIntValue("pwdLockoutDuration", req.PwdLockoutDuration, current.PwdLockoutDuration)
	updateIntValue("pwdMaxFailure", req.PwdMaxFailure, current.PwdMaxFailure)
	updateIntValue("pwdFailureCountInterval", req.PwdFailureCountInterval, current.PwdFailureCountInterval)
	updateBoolValue("pwdMustChange", req.PwdMustChange, current.PwdMustChange)
	updateBoolValue("pwdAllowUserChange", req.PwdAllowUserChange, current.PwdAllowUserChange)
	updateBoolValue("pwdSafeModify", req.PwdSafeModify, current.PwdSafeModify)
	updateStringValue("pwdCheckModule", req.PwdCheckModule, current.PwdCheckModule)

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update password policy: %w", err)
		}
	}

	return c.GetPasswordPolicy(dn)
}

// DeletePasswordPolicy deletes a password policy
func (c *Client) DeletePasswordPolicy(dn string) error {
	return c.Delete(dn)
}

func entryToPasswordPolicy(entry *ldap.Entry) PasswordPolicy {
	pwdMinAge, _ := strconv.Atoi(entry.GetAttributeValue("pwdMinAge"))
	pwdMaxAge, _ := strconv.Atoi(entry.GetAttributeValue("pwdMaxAge"))
	pwdInHistory, _ := strconv.Atoi(entry.GetAttributeValue("pwdInHistory"))
	pwdCheckQuality, _ := strconv.Atoi(entry.GetAttributeValue("pwdCheckQuality"))
	pwdMinLength, _ := strconv.Atoi(entry.GetAttributeValue("pwdMinLength"))
	pwdExpireWarning, _ := strconv.Atoi(entry.GetAttributeValue("pwdExpireWarning"))
	pwdGraceAuthNLimit, _ := strconv.Atoi(entry.GetAttributeValue("pwdGraceAuthNLimit"))
	pwdLockoutDuration, _ := strconv.Atoi(entry.GetAttributeValue("pwdLockoutDuration"))
	pwdMaxFailure, _ := strconv.Atoi(entry.GetAttributeValue("pwdMaxFailure"))
	pwdFailureCountInterval, _ := strconv.Atoi(entry.GetAttributeValue("pwdFailureCountInterval"))

	return PasswordPolicy{
		DN:                      entry.DN,
		CN:                      entry.GetAttributeValue("cn"),
		Description:             entry.GetAttributeValue("description"),
		PwdAttribute:            entry.GetAttributeValue("pwdAttribute"),
		PwdMinAge:               pwdMinAge,
		PwdMaxAge:               pwdMaxAge,
		PwdInHistory:            pwdInHistory,
		PwdCheckQuality:         pwdCheckQuality,
		PwdMinLength:            pwdMinLength,
		PwdExpireWarning:        pwdExpireWarning,
		PwdGraceAuthNLimit:      pwdGraceAuthNLimit,
		PwdLockout:              parseLDAPBool(entry.GetAttributeValue("pwdLockout")),
		PwdLockoutDuration:      pwdLockoutDuration,
		PwdMaxFailure:           pwdMaxFailure,
		PwdFailureCountInterval: pwdFailureCountInterval,
		PwdMustChange:           parseLDAPBool(entry.GetAttributeValue("pwdMustChange")),
		PwdAllowUserChange:      parseLDAPBool(entry.GetAttributeValue("pwdAllowUserChange")),
		PwdSafeModify:           parseLDAPBool(entry.GetAttributeValue("pwdSafeModify")),
		PwdCheckModule:          entry.GetAttributeValue("pwdCheckModule"),
	}
}

func parseLDAPBool(val string) bool {
	return val == "TRUE" || val == "true" || val == "1"
}
