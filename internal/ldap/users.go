package ldap

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type User struct {
	DN              string            `json:"dn"`
	UID             string            `json:"uid"`
	CN              string            `json:"cn"`
	GivenName       string            `json:"givenName,omitempty"`
	SN              string            `json:"sn"`
	DisplayName     string            `json:"displayName,omitempty"`
	Mail            string            `json:"mail,omitempty"`
	TelephoneNumber string            `json:"telephoneNumber,omitempty"`
	Title           string            `json:"title,omitempty"`
	Department      string            `json:"departmentNumber,omitempty"`
	Organization    string            `json:"o,omitempty"`
	EmployeeNumber  string            `json:"employeeNumber,omitempty"`
	EmployeeType    string            `json:"employeeType,omitempty"`
	Initials        string            `json:"initials,omitempty"`
	Manager         string            `json:"manager,omitempty"`
	UIDNumber       int               `json:"uidNumber,omitempty"`
	GIDNumber       int               `json:"gidNumber,omitempty"`
	HomeDirectory   string            `json:"homeDirectory,omitempty"`
	LoginShell      string            `json:"loginShell,omitempty"`
	Gecos           string            `json:"gecos,omitempty"`
	Description     string            `json:"description,omitempty"`
	JpegPhoto       string            `json:"jpegPhoto,omitempty"`
	SSHPublicKeys   []string          `json:"sshPublicKey,omitempty"`
	HasPassword     bool              `json:"hasPassword"`
	AccountLocked   bool              `json:"accountLocked"`
	ObjectClasses   []string          `json:"objectClasses"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	// Samba attributes (sambaSamAccount)
	SambaSID             string `json:"sambaSID,omitempty"`
	SambaPrimaryGroupSID string `json:"sambaPrimaryGroupSID,omitempty"`
	SambaAcctFlags       string `json:"sambaAcctFlags,omitempty"`
	SambaHomePath        string `json:"sambaHomePath,omitempty"`
	SambaHomeDrive       string `json:"sambaHomeDrive,omitempty"`
	SambaLogonScript     string `json:"sambaLogonScript,omitempty"`
	SambaProfilePath     string `json:"sambaProfilePath,omitempty"`
	SambaDomainName      string `json:"sambaDomainName,omitempty"`
	SambaPwdLastSet      string `json:"sambaPwdLastSet,omitempty"`
	SambaPwdCanChange    string `json:"sambaPwdCanChange,omitempty"`
	SambaPwdMustChange   string `json:"sambaPwdMustChange,omitempty"`
	SambaKickoffTime     string `json:"sambaKickoffTime,omitempty"`
	// Shadow attributes (shadowAccount)
	ShadowLastChange int `json:"shadowLastChange,omitempty"`
	ShadowMin        int `json:"shadowMin,omitempty"`
	ShadowMax        int `json:"shadowMax,omitempty"`
	ShadowWarning    int `json:"shadowWarning,omitempty"`
	ShadowInactive   int `json:"shadowInactive,omitempty"`
	ShadowExpire     int `json:"shadowExpire,omitempty"`
	ShadowFlag       int `json:"shadowFlag,omitempty"`
	// Password Policy operational attributes (ppolicy overlay)
	PwdAccountLockedTime string   `json:"pwdAccountLockedTime,omitempty"`
	PwdFailureTime       []string `json:"pwdFailureTime,omitempty"`
	PwdChangedTime       string   `json:"pwdChangedTime,omitempty"`
	PwdGraceUseTime      []string `json:"pwdGraceUseTime,omitempty"`
	PwdReset             bool     `json:"pwdReset"`
	PwdPolicySubentry    string   `json:"pwdPolicySubentry,omitempty"`
}

type CreateUserRequest struct {
	UID             string   `json:"uid"`
	GivenName       string   `json:"givenName"`
	SN              string   `json:"sn"`
	CN              string   `json:"cn,omitempty"`
	DisplayName     string   `json:"displayName,omitempty"`
	Mail            string   `json:"mail,omitempty"`
	TelephoneNumber string   `json:"telephoneNumber,omitempty"`
	Title           string   `json:"title,omitempty"`
	Department      string   `json:"departmentNumber,omitempty"`
	Organization    string   `json:"o,omitempty"`
	EmployeeNumber  string   `json:"employeeNumber,omitempty"`
	EmployeeType    string   `json:"employeeType,omitempty"`
	Initials        string   `json:"initials,omitempty"`
	Manager         string   `json:"manager,omitempty"`
	UIDNumber       int      `json:"uidNumber"`
	GIDNumber       int      `json:"gidNumber"`
	HomeDirectory   string   `json:"homeDirectory,omitempty"`
	LoginShell      string   `json:"loginShell,omitempty"`
	Gecos           string   `json:"gecos,omitempty"`
	Password        string   `json:"password,omitempty"`
	Description     string   `json:"description,omitempty"`
	ObjectClasses   []string `json:"objectClasses,omitempty"`
	Groups              []string `json:"groups,omitempty"`              // Group CNs to add the user to
	CreatePrimaryGroup  bool     `json:"createPrimaryGroup,omitempty"`  // If true, create a posixGroup with CN=UID and the given gidNumber
	ExpirationDate      string   `json:"expirationDate,omitempty"`      // ISO date string for account expiration (stored in pwdAccountLockedTime)
}

type UpdateUserRequest struct {
	GivenName       *string `json:"givenName,omitempty"`
	SN              *string `json:"sn,omitempty"`
	CN              *string `json:"cn,omitempty"`
	DisplayName     *string `json:"displayName,omitempty"`
	Mail            *string `json:"mail,omitempty"`
	TelephoneNumber *string `json:"telephoneNumber,omitempty"`
	Title           *string `json:"title,omitempty"`
	Department      *string `json:"departmentNumber,omitempty"`
	Organization    *string `json:"o,omitempty"`
	EmployeeNumber  *string `json:"employeeNumber,omitempty"`
	EmployeeType    *string `json:"employeeType,omitempty"`
	Initials        *string `json:"initials,omitempty"`
	Manager         *string `json:"manager,omitempty"`
	HomeDirectory   *string `json:"homeDirectory,omitempty"`
	LoginShell      *string `json:"loginShell,omitempty"`
	Gecos           *string `json:"gecos,omitempty"`
	Password          *string `json:"password,omitempty"`
	Description       *string `json:"description,omitempty"`
	JpegPhoto         *string `json:"jpegPhoto,omitempty"`
	PwdPolicySubentry *string `json:"pwdPolicySubentry,omitempty"`
}

var defaultUserAttributes = []string{
	"dn", "uid", "cn", "sn", "givenName", "displayName", "mail",
	"telephoneNumber", "title", "departmentNumber", "o", "employeeNumber", "employeeType",
	"initials", "manager", "uidNumber", "gidNumber",
	"homeDirectory", "loginShell", "gecos", "description", "jpegPhoto", "objectClass",
	"sshPublicKey", "userPassword",
	// Samba attributes
	"sambaSID", "sambaPrimaryGroupSID", "sambaAcctFlags", "sambaHomePath",
	"sambaHomeDrive", "sambaLogonScript", "sambaProfilePath", "sambaDomainName",
	"sambaPwdLastSet", "sambaPwdCanChange", "sambaPwdMustChange", "sambaKickoffTime",
	// Shadow attributes
	"shadowLastChange", "shadowMin", "shadowMax", "shadowWarning",
	"shadowInactive", "shadowExpire", "shadowFlag",
	// Password policy operational attributes
	"pwdAccountLockedTime", "pwdFailureTime", "pwdChangedTime",
	"pwdGraceUseTime", "pwdReset", "pwdPolicySubentry",
}

func (c *Client) ListUsers() ([]User, error) {
	entries, err := c.Search(c.UserBaseDN(), "(objectClass=inetOrgPerson)", defaultUserAttributes)
	if err != nil {
		return nil, err
	}

	users := make([]User, 0, len(entries))
	for _, entry := range entries {
		users = append(users, entryToUser(entry))
	}

	return users, nil
}

func (c *Client) GetUser(dn string) (*User, error) {
	entry, err := c.GetEntry(dn, defaultUserAttributes)
	if err != nil {
		return nil, err
	}

	user := entryToUser(entry)
	return &user, nil
}

func (c *Client) GetUserByUID(uid string) (*User, error) {
	entries, err := c.Search(c.UserBaseDN(), fmt.Sprintf("(uid=%s)", ldap.EscapeFilter(uid)), defaultUserAttributes)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("user not found: %s", uid)
	}

	user := entryToUser(entries[0])
	return &user, nil
}

func (c *Client) CreateUser(req CreateUserRequest) (*User, error) {
	if req.CN == "" {
		req.CN = fmt.Sprintf("%s %s", req.GivenName, req.SN)
	}
	if req.DisplayName == "" {
		req.DisplayName = req.CN
	}
	if req.HomeDirectory == "" {
		req.HomeDirectory = fmt.Sprintf("/home/%s", req.UID)
	}
	if req.LoginShell == "" {
		req.LoginShell = "/bin/bash"
	}

	objectClasses := req.ObjectClasses
	if len(objectClasses) == 0 {
		objectClasses = []string{"inetOrgPerson", "posixAccount", "shadowAccount"}
	}

	dn := fmt.Sprintf("uid=%s,%s", ldap.EscapeDN(req.UID), c.UserBaseDN())

	addReq := ldap.NewAddRequest(dn, nil)
	addReq.Attribute("objectClass", objectClasses)
	addReq.Attribute("uid", []string{req.UID})
	addReq.Attribute("cn", []string{req.CN})
	addReq.Attribute("sn", []string{req.SN})
	addReq.Attribute("givenName", []string{req.GivenName})
	addReq.Attribute("displayName", []string{req.DisplayName})
	addReq.Attribute("uidNumber", []string{strconv.Itoa(req.UIDNumber)})
	addReq.Attribute("gidNumber", []string{strconv.Itoa(req.GIDNumber)})
	addReq.Attribute("homeDirectory", []string{req.HomeDirectory})
	addReq.Attribute("loginShell", []string{req.LoginShell})

	if req.Mail != "" {
		addReq.Attribute("mail", []string{req.Mail})
	}
	if req.TelephoneNumber != "" {
		addReq.Attribute("telephoneNumber", []string{req.TelephoneNumber})
	}
	if req.Title != "" {
		addReq.Attribute("title", []string{req.Title})
	}
	if req.Department != "" {
		addReq.Attribute("departmentNumber", []string{req.Department})
	}
	if req.Organization != "" {
		addReq.Attribute("o", []string{req.Organization})
	}
	if req.EmployeeNumber != "" {
		addReq.Attribute("employeeNumber", []string{req.EmployeeNumber})
	}
	if req.EmployeeType != "" {
		addReq.Attribute("employeeType", []string{req.EmployeeType})
	}
	if req.Initials != "" {
		addReq.Attribute("initials", []string{req.Initials})
	}
	if req.Manager != "" {
		addReq.Attribute("manager", []string{req.Manager})
	}
	if req.Gecos != "" {
		addReq.Attribute("gecos", []string{req.Gecos})
	}
	if req.Description != "" {
		addReq.Attribute("description", []string{req.Description})
	}
	// userPassword is intentionally NOT set in the Add: a plain Add of the
	// attribute bypasses the ppolicy hashing pipeline on most server
	// configurations. The password is set in a separate PasswordModify
	// extended operation below, which triggers hashing and policy hooks.

	if err := c.Add(addReq); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if req.Password != "" {
		if err := c.PasswordModify(dn, req.Password); err != nil {
			return nil, fmt.Errorf("set initial password: %w", err)
		}
	}

	// Create a primary group with CN=UID if requested
	if req.CreatePrimaryGroup {
		groupDesc := req.Description
		if groupDesc == "" {
			groupDesc = fmt.Sprintf("Primary group for %s", req.DisplayName)
		}
		_, err := c.CreateGroup(CreateGroupRequest{
			CN:          req.UID,
			GIDNumber:   req.GIDNumber,
			Description: groupDesc,
		})
		if err != nil {
			return nil, fmt.Errorf("create primary group: %w", err)
		}
	}

	// Add user to specified groups
	if len(req.Groups) > 0 {
		for _, groupCN := range req.Groups {
			// Find the group by CN
			groupDN := fmt.Sprintf("cn=%s,%s", ldap.EscapeDN(groupCN), c.GroupBaseDN())
			if err := c.AddGroupMember(groupDN, req.UID); err != nil {
				// Log but don't fail user creation
				// The user is created, just group membership failed
				continue
			}
		}
	}

	// Set expiration date if provided (stored in pwdAccountLockedTime)
	if req.ExpirationDate != "" {
		// Parse the date and convert to LDAP generalized time format
		// Expected input format: YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS
		expirationTime, err := time.Parse("2006-01-02", req.ExpirationDate)
		if err != nil {
			// Try parsing with time component
			expirationTime, err = time.Parse("2006-01-02T15:04:05", req.ExpirationDate)
			if err != nil {
				// Try ISO 8601 format
				expirationTime, err = time.Parse(time.RFC3339, req.ExpirationDate)
			}
		}
		if err == nil {
			// Convert to LDAP generalized time format (end of day in UTC)
			if expirationTime.Hour() == 0 && expirationTime.Minute() == 0 && expirationTime.Second() == 0 {
				// If only date provided, set to end of day
				expirationTime = expirationTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			}
			ldapTime := expirationTime.UTC().Format("20060102150405Z")
			modReq := ldap.NewModifyRequest(dn, nil)
			modReq.Add("pwdAccountLockedTime", []string{ldapTime})
			// Ignore error - ppolicy may not be configured
			_ = c.Modify(modReq)
		}
	}

	return c.GetUser(dn)
}

func (c *Client) UpdateUser(dn string, req UpdateUserRequest) (*User, error) {
	// First, get the current entry to check which attributes exist
	currentUser, err := c.GetUser(dn)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	// Helper to check if an attribute has a value in the current entry
	hasAttribute := func(attr string) bool {
		switch attr {
		case "sn":
			return currentUser.SN != ""
		case "givenName":
			return currentUser.GivenName != ""
		case "cn":
			return currentUser.CN != ""
		case "displayName":
			return currentUser.DisplayName != ""
		case "mail":
			return currentUser.Mail != ""
		case "telephoneNumber":
			return currentUser.TelephoneNumber != ""
		case "title":
			return currentUser.Title != ""
		case "departmentNumber":
			return currentUser.Department != ""
		case "o":
			return currentUser.Organization != ""
		case "employeeNumber":
			return currentUser.EmployeeNumber != ""
		case "employeeType":
			return currentUser.EmployeeType != ""
		case "initials":
			return currentUser.Initials != ""
		case "manager":
			return currentUser.Manager != ""
		case "homeDirectory":
			return currentUser.HomeDirectory != ""
		case "loginShell":
			return currentUser.LoginShell != ""
		case "gecos":
			return currentUser.Gecos != ""
		case "description":
			return currentUser.Description != ""
		case "jpegPhoto":
			return currentUser.JpegPhoto != ""
		case "pwdPolicySubentry":
			return currentUser.PwdPolicySubentry != ""
		}
		return false
	}

	addModify := func(attr string, value *string) {
		if value != nil {
			if *value == "" {
				// Only delete if the attribute currently exists
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

	addModify("sn", req.SN)
	addModify("givenName", req.GivenName)
	addModify("cn", req.CN)
	addModify("displayName", req.DisplayName)
	addModify("mail", req.Mail)
	addModify("telephoneNumber", req.TelephoneNumber)
	addModify("title", req.Title)
	addModify("departmentNumber", req.Department)
	addModify("o", req.Organization)
	addModify("employeeNumber", req.EmployeeNumber)
	addModify("employeeType", req.EmployeeType)
	addModify("initials", req.Initials)
	addModify("manager", req.Manager)
	addModify("homeDirectory", req.HomeDirectory)
	addModify("loginShell", req.LoginShell)
	addModify("gecos", req.Gecos)
	addModify("description", req.Description)
	addModify("pwdPolicySubentry", req.PwdPolicySubentry)

	// req.Password is handled out-of-band via PasswordModify after the
	// regular Modify completes — see below — so the ppolicy overlay can
	// hash and validate the value rather than seeing a raw replacement.

	// Handle jpegPhoto (binary data sent as base64)
	if req.JpegPhoto != nil {
		if *req.JpegPhoto == "" {
			// Delete photo if empty and exists
			if hasAttribute("jpegPhoto") {
				modReq.Delete("jpegPhoto", nil)
				hasChanges = true
			}
		} else {
			// Decode base64 and set binary data
			photoBytes, err := base64.StdEncoding.DecodeString(*req.JpegPhoto)
			if err != nil {
				return nil, fmt.Errorf("invalid jpegPhoto base64: %w", err)
			}
			modReq.Replace("jpegPhoto", []string{string(photoBytes)})
			hasChanges = true
		}
	}

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
	}

	if req.Password != nil && *req.Password != "" {
		if err := c.PasswordModify(dn, *req.Password); err != nil {
			return nil, fmt.Errorf("update password: %w", err)
		}
	}

	return c.GetUser(dn)
}

func (c *Client) DeleteUser(dn string) error {
	return c.Delete(dn)
}

// LockUser locks a user account by setting pwdAccountLockedTime (ppolicy).
// A past timestamp tells the server to deny binds and marks the account as
// locked in the UI. The previous implementation also prefixed userPassword
// with "!"; that path was dropped because under olcPPolicyHashCleartext the
// server re-hashes the prefixed value, making the original password
// unrecoverable on unlock.
func (c *Client) LockUser(dn string) error {
	entry, err := c.GetEntry(dn, []string{"pwdAccountLockedTime"})
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	existing := entry.GetAttributeValue("pwdAccountLockedTime")
	now := time.Now().UTC().Format("20060102150405Z")

	modReq := ldap.NewModifyRequest(dn, nil)
	if existing == "" {
		modReq.Add("pwdAccountLockedTime", []string{now})
	} else {
		// Replace whether the existing value is a past lock (idempotent) or a
		// future expiration (locking now supersedes the schedule).
		modReq.Replace("pwdAccountLockedTime", []string{now})
	}
	return c.Modify(modReq)
}

// UnlockUser removes a manual lock by deleting pwdAccountLockedTime. If the
// attribute holds a future timestamp it is a scheduled expiration set via
// SetUserExpiration, not a lock, so we refuse rather than silently destroying
// the schedule. userPassword is never touched.
func (c *Client) UnlockUser(dn string) error {
	entry, err := c.GetEntry(dn, []string{"pwdAccountLockedTime"})
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	locked := entry.GetAttributeValue("pwdAccountLockedTime")
	if locked == "" {
		return nil
	}
	if t, perr := time.Parse("20060102150405Z", locked); perr == nil && t.After(time.Now()) {
		return fmt.Errorf("account has a scheduled expiration, not a manual lock; clear it via the expiration endpoint")
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	modReq.Delete("pwdAccountLockedTime", []string{})
	return c.Modify(modReq)
}

// SetUserExpiration sets or clears the account expiration date (pwdAccountLockedTime)
// If expirationDate is empty, the expiration is cleared
func (c *Client) SetUserExpiration(dn string, expirationDate string) error {
	// Check if the attribute currently exists
	entry, err := c.GetEntry(dn, []string{"pwdAccountLockedTime"})
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	hasExpiration := entry.GetAttributeValue("pwdAccountLockedTime") != ""

	modReq := ldap.NewModifyRequest(dn, nil)

	if expirationDate == "" {
		// Clear the expiration
		if hasExpiration {
			modReq.Delete("pwdAccountLockedTime", []string{})
		} else {
			return nil // Nothing to clear
		}
	} else {
		// Parse the date and convert to LDAP generalized time format
		expirationTime, err := time.Parse("2006-01-02", expirationDate)
		if err != nil {
			expirationTime, err = time.Parse("2006-01-02T15:04:05", expirationDate)
			if err != nil {
				expirationTime, err = time.Parse(time.RFC3339, expirationDate)
				if err != nil {
					return fmt.Errorf("invalid date format: %w", err)
				}
			}
		}

		// If only date provided, set to end of day
		if expirationTime.Hour() == 0 && expirationTime.Minute() == 0 && expirationTime.Second() == 0 {
			expirationTime = expirationTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		}
		ldapTime := expirationTime.UTC().Format("20060102150405Z")

		if hasExpiration {
			modReq.Replace("pwdAccountLockedTime", []string{ldapTime})
		} else {
			modReq.Add("pwdAccountLockedTime", []string{ldapTime})
		}
	}

	return c.Modify(modReq)
}

// SetSSHPublicKeys replaces all SSH public keys for a user
func (c *Client) SetSSHPublicKeys(dn string, keys []string) error {
	// First check if user has ldapPublicKey objectClass
	user, err := c.GetUser(dn)
	if err != nil {
		return err
	}

	hasObjectClass := false
	for _, oc := range user.ObjectClasses {
		if oc == "ldapPublicKey" {
			hasObjectClass = true
			break
		}
	}

	modReq := ldap.NewModifyRequest(dn, nil)

	// Add objectClass if not present and we have keys to add
	if !hasObjectClass && len(keys) > 0 {
		modReq.Add("objectClass", []string{"ldapPublicKey"})
	}

	if len(keys) > 0 {
		if len(user.SSHPublicKeys) > 0 {
			modReq.Replace("sshPublicKey", keys)
		} else {
			modReq.Add("sshPublicKey", keys)
		}
	} else if len(user.SSHPublicKeys) > 0 {
		modReq.Delete("sshPublicKey", nil)
	}

	return c.Modify(modReq)
}

// AddSSHPublicKey adds a single SSH public key to a user
func (c *Client) AddSSHPublicKey(dn string, key string) error {
	user, err := c.GetUser(dn)
	if err != nil {
		return err
	}

	hasObjectClass := false
	for _, oc := range user.ObjectClasses {
		if oc == "ldapPublicKey" {
			hasObjectClass = true
			break
		}
	}

	modReq := ldap.NewModifyRequest(dn, nil)

	if !hasObjectClass {
		modReq.Add("objectClass", []string{"ldapPublicKey"})
	}

	modReq.Add("sshPublicKey", []string{key})
	return c.Modify(modReq)
}

// RemoveSSHPublicKey removes a single SSH public key from a user
func (c *Client) RemoveSSHPublicKey(dn string, key string) error {
	modReq := ldap.NewModifyRequest(dn, nil)
	modReq.Delete("sshPublicKey", []string{key})
	return c.Modify(modReq)
}

// ChangePassword changes a user's password via the RFC 3062 PasswordModify
// extended operation. This delegates hashing to the directory and triggers
// the password-policy overlay correctly (history, quality, hashing) — a
// plain Modify of userPassword would store the value as-is unless ppolicy
// is configured with hash_cleartext.
func (c *Client) ChangePassword(dn string, newPassword string) error {
	return c.PasswordModify(dn, newPassword)
}

// RemovePassword removes the userPassword attribute, preventing the user from authenticating.
func (c *Client) RemovePassword(dn string) error {
	modReq := ldap.NewModifyRequest(dn, nil)
	modReq.Delete("userPassword", nil)
	return c.Modify(modReq)
}

// UpdateSambaUserRequest contains Samba-specific user attributes
type UpdateSambaUserRequest struct {
	SambaSID             *string `json:"sambaSID,omitempty"`
	SambaPrimaryGroupSID *string `json:"sambaPrimaryGroupSID,omitempty"`
	SambaAcctFlags       *string `json:"sambaAcctFlags,omitempty"`
	SambaHomePath        *string `json:"sambaHomePath,omitempty"`
	SambaHomeDrive       *string `json:"sambaHomeDrive,omitempty"`
	SambaLogonScript     *string `json:"sambaLogonScript,omitempty"`
	SambaProfilePath     *string `json:"sambaProfilePath,omitempty"`
	SambaDomainName      *string `json:"sambaDomainName,omitempty"`
}

// SetSambaUserAttributes updates Samba attributes for a user
func (c *Client) SetSambaUserAttributes(dn string, req UpdateSambaUserRequest) (*User, error) {
	user, err := c.GetUser(dn)
	if err != nil {
		return nil, err
	}

	// Check if user has sambaSamAccount objectClass
	hasObjectClass := false
	for _, oc := range user.ObjectClasses {
		if oc == "sambaSamAccount" {
			hasObjectClass = true
			break
		}
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	// Add objectClass if not present and we have a SID to set
	if !hasObjectClass && req.SambaSID != nil && *req.SambaSID != "" {
		modReq.Add("objectClass", []string{"sambaSamAccount"})
		hasChanges = true
	}

	// Helper to check if an attribute has a value
	hasAttribute := func(attr string) bool {
		switch attr {
		case "sambaSID":
			return user.SambaSID != ""
		case "sambaPrimaryGroupSID":
			return user.SambaPrimaryGroupSID != ""
		case "sambaAcctFlags":
			return user.SambaAcctFlags != ""
		case "sambaHomePath":
			return user.SambaHomePath != ""
		case "sambaHomeDrive":
			return user.SambaHomeDrive != ""
		case "sambaLogonScript":
			return user.SambaLogonScript != ""
		case "sambaProfilePath":
			return user.SambaProfilePath != ""
		case "sambaDomainName":
			return user.SambaDomainName != ""
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
	addModify("sambaPrimaryGroupSID", req.SambaPrimaryGroupSID)
	addModify("sambaAcctFlags", req.SambaAcctFlags)
	addModify("sambaHomePath", req.SambaHomePath)
	addModify("sambaHomeDrive", req.SambaHomeDrive)
	addModify("sambaLogonScript", req.SambaLogonScript)
	addModify("sambaProfilePath", req.SambaProfilePath)
	addModify("sambaDomainName", req.SambaDomainName)

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update samba attributes: %w", err)
		}
	}

	return c.GetUser(dn)
}

// UpdateShadowUserRequest contains shadow-specific user attributes
type UpdateShadowUserRequest struct {
	ShadowLastChange *int `json:"shadowLastChange,omitempty"`
	ShadowMin        *int `json:"shadowMin,omitempty"`
	ShadowMax        *int `json:"shadowMax,omitempty"`
	ShadowWarning    *int `json:"shadowWarning,omitempty"`
	ShadowInactive   *int `json:"shadowInactive,omitempty"`
	ShadowExpire     *int `json:"shadowExpire,omitempty"`
	ShadowFlag       *int `json:"shadowFlag,omitempty"`
}

// SetShadowUserAttributes updates shadow attributes for a user
func (c *Client) SetShadowUserAttributes(dn string, req UpdateShadowUserRequest) (*User, error) {
	user, err := c.GetUser(dn)
	if err != nil {
		return nil, err
	}

	// Check if user has shadowAccount objectClass
	hasObjectClass := false
	for _, oc := range user.ObjectClasses {
		if oc == "shadowAccount" {
			hasObjectClass = true
			break
		}
	}

	modReq := ldap.NewModifyRequest(dn, nil)
	hasChanges := false

	// Add objectClass if not present and we have attributes to set
	needsObjectClass := !hasObjectClass && (req.ShadowLastChange != nil || req.ShadowMin != nil ||
		req.ShadowMax != nil || req.ShadowWarning != nil || req.ShadowInactive != nil ||
		req.ShadowExpire != nil || req.ShadowFlag != nil)
	if needsObjectClass {
		modReq.Add("objectClass", []string{"shadowAccount"})
		hasChanges = true
	}

	// Helper to check if an attribute has a value
	hasAttribute := func(attr string) bool {
		switch attr {
		case "shadowLastChange":
			return user.ShadowLastChange != 0
		case "shadowMin":
			return user.ShadowMin != 0
		case "shadowMax":
			return user.ShadowMax != 0
		case "shadowWarning":
			return user.ShadowWarning != 0
		case "shadowInactive":
			return user.ShadowInactive != 0
		case "shadowExpire":
			return user.ShadowExpire != 0
		case "shadowFlag":
			return user.ShadowFlag != 0
		}
		return false
	}

	// Helper to modify integer attributes
	// Special case: -1 means delete the attribute, 0 could be a valid value
	addModifyInt := func(attr string, value *int) {
		if value != nil {
			if *value == -1 {
				// Delete the attribute
				if hasAttribute(attr) {
					modReq.Delete(attr, nil)
					hasChanges = true
				}
			} else {
				modReq.Replace(attr, []string{strconv.Itoa(*value)})
				hasChanges = true
			}
		}
	}

	addModifyInt("shadowLastChange", req.ShadowLastChange)
	addModifyInt("shadowMin", req.ShadowMin)
	addModifyInt("shadowMax", req.ShadowMax)
	addModifyInt("shadowWarning", req.ShadowWarning)
	addModifyInt("shadowInactive", req.ShadowInactive)
	addModifyInt("shadowExpire", req.ShadowExpire)
	addModifyInt("shadowFlag", req.ShadowFlag)

	if hasChanges {
		if err := c.Modify(modReq); err != nil {
			return nil, fmt.Errorf("update shadow attributes: %w", err)
		}
	}

	return c.GetUser(dn)
}

func entryToUser(entry *ldap.Entry) User {
	uidNumber, _ := strconv.Atoi(entry.GetAttributeValue("uidNumber"))
	gidNumber, _ := strconv.Atoi(entry.GetAttributeValue("gidNumber"))

	// jpegPhoto is binary, encode as base64
	var jpegPhoto string
	if photoBytes := entry.GetRawAttributeValue("jpegPhoto"); len(photoBytes) > 0 {
		jpegPhoto = base64.StdEncoding.EncodeToString(photoBytes)
	}

	// Check if user has a password set
	hasPassword := entry.GetAttributeValue("userPassword") != ""

	// Account is locked when pwdAccountLockedTime is set to a past timestamp.
	// A future value represents a scheduled expiration set via
	// SetUserExpiration (and is surfaced separately via PwdAccountLockedTime).
	// An unparseable value (including the ppolicy "000001010000Z" permanent-
	// lock sentinel) is treated as locked.
	var accountLocked bool
	if locked := entry.GetAttributeValue("pwdAccountLockedTime"); locked != "" {
		if t, err := time.Parse("20060102150405Z", locked); err == nil {
			accountLocked = !t.After(time.Now())
		} else {
			accountLocked = true
		}
	}

	// Shadow attributes (all integers)
	shadowLastChange, _ := strconv.Atoi(entry.GetAttributeValue("shadowLastChange"))
	shadowMin, _ := strconv.Atoi(entry.GetAttributeValue("shadowMin"))
	shadowMax, _ := strconv.Atoi(entry.GetAttributeValue("shadowMax"))
	shadowWarning, _ := strconv.Atoi(entry.GetAttributeValue("shadowWarning"))
	shadowInactive, _ := strconv.Atoi(entry.GetAttributeValue("shadowInactive"))
	shadowExpire, _ := strconv.Atoi(entry.GetAttributeValue("shadowExpire"))
	shadowFlag, _ := strconv.Atoi(entry.GetAttributeValue("shadowFlag"))

	return User{
		DN:              entry.DN,
		UID:             entry.GetAttributeValue("uid"),
		CN:              entry.GetAttributeValue("cn"),
		GivenName:       entry.GetAttributeValue("givenName"),
		SN:              entry.GetAttributeValue("sn"),
		DisplayName:     entry.GetAttributeValue("displayName"),
		Mail:            entry.GetAttributeValue("mail"),
		TelephoneNumber: entry.GetAttributeValue("telephoneNumber"),
		Title:           entry.GetAttributeValue("title"),
		Department:      entry.GetAttributeValue("departmentNumber"),
		Organization:    entry.GetAttributeValue("o"),
		EmployeeNumber:  entry.GetAttributeValue("employeeNumber"),
		EmployeeType:    entry.GetAttributeValue("employeeType"),
		Initials:        entry.GetAttributeValue("initials"),
		Manager:         entry.GetAttributeValue("manager"),
		UIDNumber:       uidNumber,
		GIDNumber:       gidNumber,
		HomeDirectory:   entry.GetAttributeValue("homeDirectory"),
		LoginShell:      entry.GetAttributeValue("loginShell"),
		Gecos:           entry.GetAttributeValue("gecos"),
		Description:     entry.GetAttributeValue("description"),
		JpegPhoto:       jpegPhoto,
		SSHPublicKeys:   entry.GetAttributeValues("sshPublicKey"),
		HasPassword:     hasPassword,
		AccountLocked:   accountLocked,
		ObjectClasses:   entry.GetAttributeValues("objectClass"),
		// Samba attributes
		SambaSID:             entry.GetAttributeValue("sambaSID"),
		SambaPrimaryGroupSID: entry.GetAttributeValue("sambaPrimaryGroupSID"),
		SambaAcctFlags:       entry.GetAttributeValue("sambaAcctFlags"),
		SambaHomePath:        entry.GetAttributeValue("sambaHomePath"),
		SambaHomeDrive:       entry.GetAttributeValue("sambaHomeDrive"),
		SambaLogonScript:     entry.GetAttributeValue("sambaLogonScript"),
		SambaProfilePath:     entry.GetAttributeValue("sambaProfilePath"),
		SambaDomainName:      entry.GetAttributeValue("sambaDomainName"),
		SambaPwdLastSet:      entry.GetAttributeValue("sambaPwdLastSet"),
		SambaPwdCanChange:    entry.GetAttributeValue("sambaPwdCanChange"),
		SambaPwdMustChange:   entry.GetAttributeValue("sambaPwdMustChange"),
		SambaKickoffTime:     entry.GetAttributeValue("sambaKickoffTime"),
		// Shadow attributes
		ShadowLastChange: shadowLastChange,
		ShadowMin:        shadowMin,
		ShadowMax:        shadowMax,
		ShadowWarning:    shadowWarning,
		ShadowInactive:   shadowInactive,
		ShadowExpire:     shadowExpire,
		ShadowFlag:       shadowFlag,
		// Password policy operational attributes
		PwdAccountLockedTime: entry.GetAttributeValue("pwdAccountLockedTime"),
		PwdFailureTime:       entry.GetAttributeValues("pwdFailureTime"),
		PwdChangedTime:       entry.GetAttributeValue("pwdChangedTime"),
		PwdGraceUseTime:      entry.GetAttributeValues("pwdGraceUseTime"),
		PwdReset:             parseLDAPBool(entry.GetAttributeValue("pwdReset")),
		PwdPolicySubentry:    entry.GetAttributeValue("pwdPolicySubentry"),
	}
}
