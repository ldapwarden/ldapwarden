package ldap

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/go-ldap/ldap/v3"
	"github.com/ldapwarden/ldapwarden/internal/config"
)

type Client struct {
	config config.LDAPConfig
	mu     sync.Mutex
	conn   *ldap.Conn
}

func NewClient(cfg config.LDAPConfig) *Client {
	return &Client{
		config: cfg,
	}
}

func (c *Client) connect() (*ldap.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		if err := c.conn.Bind(c.config.BindDN, c.config.BindPass); err == nil {
			return c.conn, nil
		}
		_ = c.conn.Close()
	}

	var conn *ldap.Conn
	var err error

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.config.TLSSkipVerify, //nolint:gosec // User-configurable setting
	}

	switch c.config.TLSMode {
	case "ssl":
		// LDAPS - TLS from the start (typically port 636)
		conn, err = ldap.DialURL(c.config.URL, ldap.DialWithTLSConfig(tlsConfig))
	case "starttls":
		// StartTLS - connect plain, then upgrade to TLS
		conn, err = ldap.DialURL(c.config.URL)
		if err != nil {
			return nil, fmt.Errorf("dial LDAP server: %w", err)
		}
		if err = conn.StartTLS(tlsConfig); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("StartTLS: %w", err)
		}
	default:
		// "none" or unset - plain LDAP (or auto-detect ldaps:// URL)
		if strings.HasPrefix(c.config.URL, "ldaps://") {
			conn, err = ldap.DialURL(c.config.URL, ldap.DialWithTLSConfig(tlsConfig))
		} else {
			conn, err = ldap.DialURL(c.config.URL)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("dial LDAP server: %w", err)
	}

	if err := conn.Bind(c.config.BindDN, c.config.BindPass); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("bind to LDAP server: %w", err)
	}

	c.conn = conn
	return conn, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) Bind(dn, password string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	_ = conn

	// Create a separate connection for user authentication
	var testConn *ldap.Conn

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.config.TLSSkipVerify, //nolint:gosec // User-configurable setting
	}

	switch c.config.TLSMode {
	case "ssl":
		testConn, err = ldap.DialURL(c.config.URL, ldap.DialWithTLSConfig(tlsConfig))
	case "starttls":
		testConn, err = ldap.DialURL(c.config.URL)
		if err != nil {
			return fmt.Errorf("dial LDAP server for auth: %w", err)
		}
		if err = testConn.StartTLS(tlsConfig); err != nil {
			_ = testConn.Close()
			return fmt.Errorf("StartTLS for auth: %w", err)
		}
	default:
		if strings.HasPrefix(c.config.URL, "ldaps://") {
			testConn, err = ldap.DialURL(c.config.URL, ldap.DialWithTLSConfig(tlsConfig))
		} else {
			testConn, err = ldap.DialURL(c.config.URL)
		}
	}

	if err != nil {
		return fmt.Errorf("dial LDAP server for auth: %w", err)
	}
	defer func() { _ = testConn.Close() }()

	if err := testConn.Bind(dn, password); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}

func (c *Client) Search(baseDN, filter string, attributes []string) ([]*ldap.Entry, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		attributes,
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search LDAP: %w", err)
	}

	return result.Entries, nil
}

func (c *Client) GetEntry(dn string, attributes []string) (*ldap.Entry, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}

	searchRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, 0, false,
		"(objectClass=*)",
		attributes,
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("get entry: %w", err)
	}

	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("entry not found: %s", dn)
	}

	return result.Entries[0], nil
}

func (c *Client) Add(entry *ldap.AddRequest) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}

	return conn.Add(entry)
}

func (c *Client) Modify(request *ldap.ModifyRequest) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}

	return conn.Modify(request)
}

// PasswordModify issues an RFC 3062 PasswordModify extended operation against
// dn. Unlike a plain Modify on the userPassword attribute, this lets the
// server's password-policy overlay hash the value and run pre-set hooks
// (history, quality checks). The connection is bound as the configured
// BindDN, so oldPassword is empty — the server allows administrative resets
// in that mode.
func (c *Client) PasswordModify(dn, newPassword string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}
	req := ldap.NewPasswordModifyRequest(dn, "", newPassword)
	_, err = conn.PasswordModify(req)
	return err
}

func (c *Client) Delete(dn string) error {
	conn, err := c.connect()
	if err != nil {
		return err
	}

	delRequest := ldap.NewDelRequest(dn, nil)
	return conn.Del(delRequest)
}

func (c *Client) BaseDN() string {
	return c.config.BaseDN
}

func (c *Client) UserBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.UserOU, c.config.BaseDN)
}

func (c *Client) GroupBaseDN() string {
	return fmt.Sprintf("%s,%s", c.config.GroupOU, c.config.BaseDN)
}

func (c *Client) MinUID() int {
	return c.config.MinUID
}

func (c *Client) MinGID() int {
	return c.config.MinGID
}

// NextUID finds the next available UID starting from MinUID
func (c *Client) NextUID() (int, error) {
	entries, err := c.Search(c.UserBaseDN(), "(objectClass=posixAccount)", []string{"uidNumber"})
	if err != nil {
		return 0, fmt.Errorf("search for UIDs: %w", err)
	}

	usedUIDs := make(map[int]bool)
	for _, entry := range entries {
		if uidStr := entry.GetAttributeValue("uidNumber"); uidStr != "" {
			if uid, err := strconv.Atoi(uidStr); err == nil {
				usedUIDs[uid] = true
			}
		}
	}

	// Find the next available UID starting from MinUID
	nextUID := c.config.MinUID
	for usedUIDs[nextUID] {
		nextUID++
	}

	return nextUID, nil
}

// NextGID finds the next available GID starting from MinGID
func (c *Client) NextGID() (int, error) {
	entries, err := c.Search(c.GroupBaseDN(), "(objectClass=posixGroup)", []string{"gidNumber"})
	if err != nil {
		return 0, fmt.Errorf("search for GIDs: %w", err)
	}

	usedGIDs := make(map[int]bool)
	for _, entry := range entries {
		if gidStr := entry.GetAttributeValue("gidNumber"); gidStr != "" {
			if gid, err := strconv.Atoi(gidStr); err == nil {
				usedGIDs[gid] = true
			}
		}
	}

	// Find the next available GID starting from MinGID
	nextGID := c.config.MinGID
	for usedGIDs[nextGID] {
		nextGID++
	}

	return nextGID, nil
}
