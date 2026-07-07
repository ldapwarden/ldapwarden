package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/vanng822/go-premailer/premailer"
)

// errHeaderInjection is returned by buildMessage when a value destined for an
// SMTP/MIME header carries a CR, LF or NUL byte. Header lines are
// line-oriented; allowing those characters through would let an attacker
// inject extra Bcc/From headers and rewrite the message body, which matters
// because several header inputs are sourced from LDAP attributes (mail,
// displayName) or from audit-log fields (action, actorUID).
var errHeaderInjection = errors.New("mail: header value contains control characters")

// validateHeaderValue rejects \r, \n and \x00. Other control characters are
// allowed — the goal is anti-injection, not RFC 2047 encoding.
func validateHeaderValue(s string) error {
	if strings.ContainsAny(s, "\r\n\x00") {
		return errHeaderInjection
	}
	return nil
}


type Mailer struct {
	config       *config.MailConfig
	organization string
	publicURL    string
}

func NewMailer(cfg *config.MailConfig, organization, publicURL string) *Mailer {
	return &Mailer{
		config:       cfg,
		organization: organization,
		publicURL:    strings.TrimRight(publicURL, "/"),
	}
}

func (m *Mailer) SendPasswordResetEmail(to, userName, resetLink string) error {
	subject := fmt.Sprintf("Password Reset Request - %s", m.organization)

	data := map[string]string{
		"Organization": m.organization,
		"UserName":     userName,
		"ResetLink":    resetLink,
	}

	body, err := m.renderTemplate(passwordResetTemplate, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	return m.sendEmail(to, subject, body)
}

func (m *Mailer) SendPasswordChangedNotification(recipients []string, userName, changedByIP, whoisInfo string) error {
	subject := fmt.Sprintf("Password Changed - %s", m.organization)

	data := map[string]string{
		"Organization": m.organization,
		"UserName":     userName,
		"IPAddress":    changedByIP,
		"WhoisInfo":    whoisInfo,
	}

	body, err := m.renderTemplate(passwordChangedTemplate, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	for _, recipient := range recipients {
		if err := m.sendEmail(recipient, subject, body); err != nil {
			return fmt.Errorf("send to %s: %w", recipient, err)
		}
	}

	return nil
}

// SendAccountExpirationNotification sends account expiration warnings to admins
func (m *Mailer) SendAccountExpirationNotification(to, uid, displayName string, expDate time.Time, interval string) error {
	subject := fmt.Sprintf("Account Expiration Notice - %s", m.organization)

	data := map[string]string{
		"Organization":   m.organization,
		"UserName":       displayName,
		"UserUID":        uid,
		"ExpirationDate": expDate.Format("January 2, 2006 at 3:04 PM MST"),
		"Interval":       interval,
	}

	body, err := m.renderTemplate(accountExpirationTemplate, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	return m.sendEmail(to, subject, body)
}

// auditChangeView is one rendered diff row. Masked rows show the field name
// without any value (used for secrets such as passwords).
type auditChangeView struct {
	Field  string
	Old    string
	New    string
	Masked bool
}

// auditEmailData is the template model for an audit notification.
type auditEmailData struct {
	Organization string
	Subject      string
	Actor        string
	HasChanges   bool
	// IsFieldList renders Changes as a "field: value" dump (creations) rather
	// than an "old → new" diff (modifications). Field-list entries carry their
	// value in New.
	IsFieldList bool
	Changes     []auditChangeView
	Summary     string
	DetailsURL  string
	Timestamp   string
	ActorDN     string
	ResourceDN  string
	IPAddress   string
	UserAgent   string
}

// SendAuditNotification sends a per-change audit email to each recipient.
// Args are primitives (changes is a slice of plain string maps) to keep this
// package free of an internal/audit import. Each modification action recorded
// in the audit log produces one call.
func (m *Mailer) SendAuditNotification(
	recipients []string,
	timestamp time.Time,
	actorUID, actorName, actorDN string,
	action, resourceType, resourceDN, resourceName string,
	changes []map[string]string,
	details map[string]interface{},
	ipAddress, userAgent string,
) error {
	if len(recipients) == 0 {
		return nil
	}

	actor := displayActor(actorName, actorUID, actorDN)
	typeLabel := humanResourceType(resourceType)
	name := resourceName
	if name == "" {
		name = rdnValue(resourceDN)
	}

	views := toChangeViews(changes)
	subject := auditSubject(action, typeLabel, name)

	data := auditEmailData{
		Organization: m.organization,
		Subject:      subject,
		Actor:        actor,
		HasChanges:   len(views) > 0,
		IsFieldList:  strings.HasSuffix(action, ".create"),
		Changes:      views,
		Summary:      auditSummary(action, typeLabel, name, details),
		DetailsURL:   m.resourceURL(resourceType, resourceDN),
		Timestamp:    timestamp.UTC().Format("2006-01-02 15:04:05 MST"),
		ActorDN:      actorDN,
		ResourceDN:   resourceDN,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
	}

	body, err := m.renderTemplate(auditNotificationTemplate, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	for _, recipient := range recipients {
		if err := m.sendEmail(recipient, subject, body); err != nil {
			return fmt.Errorf("send to %s: %w", recipient, err)
		}
	}

	return nil
}

func toChangeViews(changes []map[string]string) []auditChangeView {
	out := make([]auditChangeView, 0, len(changes))
	for _, c := range changes {
		out = append(out, auditChangeView{
			Field:  c["field"],
			Old:    c["old"],
			New:    c["new"],
			Masked: c["masked"] == "true",
		})
	}
	return out
}

// auditSubject mirrors the human-readable subjects used elsewhere: creations,
// deletions and membership changes get their own phrasing; everything else is
// treated as a modification.
func auditSubject(action, typeLabel, name string) string {
	switch {
	case strings.HasSuffix(action, ".create"):
		return fmt.Sprintf("New %s created: %s", typeLabel, name)
	case strings.HasSuffix(action, ".delete"):
		return fmt.Sprintf("%s deleted: %s", capitalize(typeLabel), name)
	case action == "user.lock":
		return fmt.Sprintf("Account locked: %s", name)
	case action == "user.unlock":
		return fmt.Sprintf("Account unlocked: %s", name)
	case strings.Contains(action, "member"):
		return fmt.Sprintf("Membership change: %s", name)
	default:
		return fmt.Sprintf("Modification of %s", name)
	}
}

// auditSummary is the one-line description shown when there is no field-level
// diff to render (creations, deletions, membership and secret-only changes).
func auditSummary(action, typeLabel, name string, details map[string]interface{}) string {
	switch {
	case strings.HasSuffix(action, ".create"):
		return fmt.Sprintf("A new %s was created: %s.", typeLabel, name)
	case strings.HasSuffix(action, ".delete"):
		return fmt.Sprintf("The %s %s was deleted.", typeLabel, name)
	case action == "user.lock":
		return fmt.Sprintf("The account %s was locked.", name)
	case action == "user.unlock":
		return fmt.Sprintf("The account %s was unlocked.", name)
	case action == "group.member.add":
		return fmt.Sprintf("%s was added to the group %s.", detailString(details, "memberUid"), name)
	case action == "group.member.remove":
		return fmt.Sprintf("%s was removed from the group %s.", detailString(details, "memberUid"), name)
	}
	if a := detailString(details, "action"); a != "" {
		return fmt.Sprintf("%s was updated (%s).", name, humanizeToken(a))
	}
	return fmt.Sprintf("%s was modified.", name)
}

// resourceURL builds a link to the resource's page in the web UI, matching the
// frontend's base64url DN encoding (encodeDN). Returns "" when no public URL is
// configured or the resource type has no detail page.
func (m *Mailer) resourceURL(resourceType, dn string) string {
	if m.publicURL == "" || dn == "" {
		return ""
	}
	var seg string
	switch resourceType {
	case "user":
		seg = "users"
	case "group":
		seg = "groups"
	default:
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", m.publicURL, seg, base64.RawURLEncoding.EncodeToString([]byte(dn)))
}

// humanResourceType turns an audit resource type into a label for prose.
func humanResourceType(t string) string {
	switch t {
	case "user":
		return "account"
	case "group":
		return "group"
	case "sudorole":
		return "sudo role"
	case "pwdpolicy":
		return "password policy"
	default:
		if t == "" {
			return "resource"
		}
		return t
	}
}

// rdnValue returns the value of the first RDN of a DN (uid=jdoe,ou=… → jdoe),
// used as a fallback display name when no resourceName was supplied.
func rdnValue(dn string) string {
	if dn == "" {
		return "unknown"
	}
	first := dn
	if i := strings.IndexByte(dn, ','); i >= 0 {
		first = dn[:i]
	}
	if i := strings.IndexByte(first, '='); i >= 0 {
		return strings.TrimSpace(first[i+1:])
	}
	return strings.TrimSpace(first)
}

// humanizeToken turns a snake_case action token into a readable phrase
// ("samba_update" → "samba update").
func humanizeToken(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func detailString(details map[string]interface{}, key string) string {
	if v, ok := details[key].(string); ok {
		return v
	}
	return ""
}

// displayActor formats the actor for the notification: the friendly name
// qualified with the uid ("Lionel Porcheron (lionel)") when a display name is
// available, otherwise the bare uid, then the DN, then "unknown".
func displayActor(name, uid, dn string) string {
	if uid != "" {
		return nameWithUID(name, uid)
	}
	if dn != "" {
		return dn
	}
	return "unknown"
}

// nameWithUID renders "name (uid)" when name is set and distinct from uid,
// otherwise the bare uid. Mirrors the resource-name format built by the API
// layer (this package deliberately stays free of an api/audit import).
func nameWithUID(name, uid string) string {
	if name == "" || name == uid {
		return uid
	}
	return fmt.Sprintf("%s (%s)", name, uid)
}

// SendPasswordExpirationNotification sends password expiration warnings to users
func (m *Mailer) SendPasswordExpirationNotification(to, uid, displayName string, expDate time.Time, interval string) error {
	subject := fmt.Sprintf("Password Expiration Notice - %s", m.organization)

	data := map[string]string{
		"Organization":   m.organization,
		"UserName":       displayName,
		"UserUID":        uid,
		"ExpirationDate": expDate.Format("January 2, 2006 at 3:04 PM MST"),
		"Interval":       interval,
	}

	body, err := m.renderTemplate(passwordExpirationTemplate, data)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	return m.sendEmail(to, subject, body)
}

func (m *Mailer) sendEmail(to, subject, htmlBody string) error {
	if m.config.Host == "" {
		// Skip sending if mail is not configured
		return nil
	}

	msg, err := m.buildMessage(to, subject, htmlBody)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)

	switch m.config.SSL {
	case "ssl":
		return m.sendWithSSL(addr, to, msg)
	case "starttls":
		return m.sendWithStartTLS(addr, to, msg)
	default: // "none"
		return m.sendPlain(addr, to, msg)
	}
}

func (m *Mailer) buildMessage(to, subject, htmlBody string) ([]byte, error) {
	for _, h := range []string{m.config.From, m.config.FromName, to, subject} {
		if err := validateHeaderValue(h); err != nil {
			return nil, err
		}
	}
	// Only the From header carries the optional display name ("Name <addr>");
	// mail.Address.String() handles quoting and RFC 2047 encoding of the name.
	// The SMTP envelope sender (client.Mail) always uses the bare address.
	fromHeader := m.config.From
	if m.config.FromName != "" {
		fromHeader = (&mail.Address{Name: m.config.FromName, Address: m.config.From}).String()
	}
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	return msg.Bytes(), nil
}

// sendPlain sends email without any TLS
func (m *Mailer) sendPlain(addr, to string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, m.config.Host)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}
	defer func() { _ = client.Close() }()

	return m.sendWithClient(client, to, msg, false)
}

// sendWithStartTLS sends email using STARTTLS upgrade
func (m *Mailer) sendWithStartTLS(addr, to string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, m.config.Host)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}
	defer func() { _ = client.Close() }()

	// MAIL_SSL=starttls is an explicit request for an encrypted channel. If the
	// server does not advertise STARTTLS, refuse to send rather than silently
	// downgrading to cleartext — otherwise an active MITM that strips the
	// advertisement would harvest the message and any SMTP AUTH credentials.
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("starttls: server does not advertise STARTTLS; refusing to send unencrypted")
	}
	tlsConfig := &tls.Config{
		ServerName: m.config.Host,
		MinVersion: tls.VersionTLS12,
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	return m.sendWithClient(client, to, msg, true)
}

// sendWithSSL sends email over implicit TLS connection
func (m *Mailer) sendWithSSL(addr, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName: m.config.Host,
		MinVersion: tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, m.config.Host)
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}
	defer func() { _ = client.Close() }()

	return m.sendWithClient(client, to, msg, true)
}

// unencryptedAuth is like smtp.PlainAuth but allows unencrypted connections
// Use only when MAIL_SSL=none is explicitly configured
type unencryptedAuth struct {
	identity, username, password, host string
}

func (a *unencryptedAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *unencryptedAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, fmt.Errorf("unexpected server challenge")
	}
	return nil, nil
}

// sendWithClient sends the email using an established SMTP client
func (m *Mailer) sendWithClient(client *smtp.Client, to string, msg []byte, encrypted bool) error {
	// Authenticate if credentials are provided
	if m.config.User != "" && m.config.Password != "" {
		var auth smtp.Auth
		if encrypted {
			// Use standard PlainAuth for encrypted connections
			auth = smtp.PlainAuth("", m.config.User, m.config.Password, m.config.Host)
		} else {
			// Use custom auth that allows unencrypted connections
			auth = &unencryptedAuth{
				identity: "",
				username: m.config.User,
				password: m.config.Password,
				host:     m.config.Host,
			}
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(m.config.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}

	// Set recipient
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}

	// Send message body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	return client.Quit()
}

func (m *Mailer) renderTemplate(tmpl string, data interface{}) (string, error) {
	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return inlineCSS(buf.String()), nil
}

// inlineCSS rewrites the <style> rules of an HTML email into inline style
// attributes so the styling survives mail clients that strip the <head>
// (Outlook's Word engine, several Gmail contexts). On any premailer error the
// original HTML is returned unchanged — a degraded but still-readable email
// beats no email at all.
func inlineCSS(html string) string {
	prem, err := premailer.NewPremailerFromString(html, premailer.NewOptions())
	if err != nil {
		log.Printf("mail: premailer init failed, sending non-inlined HTML: %v", err)
		return html
	}
	out, err := prem.Transform()
	if err != nil {
		log.Printf("mail: premailer transform failed, sending non-inlined HTML: %v", err)
		return html
	}
	return out
}

const passwordResetTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #1a1a2e; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; }
        .button { display: inline-block; background: #3b82f6; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; margin: 20px 0; }
        .warning { background: #fef3cd; border: 1px solid #ffc107; padding: 12px; border-radius: 6px; margin-top: 20px; }
        .footer { margin-top: 20px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <h2 style="margin: 0;">{{.Organization}}</h2>
    </div>
    <div class="content">
        <p>Hello {{.UserName}},</p>
        <p>A password reset has been requested for your account. Click the button below to set a new password:</p>
        <p style="text-align: center;">
            <a href="{{.ResetLink}}" class="button">Reset Password</a>
        </p>
        <p>Or copy and paste this link into your browser:</p>
        <p style="word-break: break-all; background: #e9ecef; padding: 10px; border-radius: 4px; font-size: 14px;">{{.ResetLink}}</p>
        <p><strong>This link will expire in 24 hours.</strong></p>
        <div class="warning">
            <strong>Didn't request this?</strong><br>
            If you didn't request a password reset, you can safely ignore this email. Your password will not be changed.
        </div>
        <div class="footer">
            <p>This is an automated message from {{.Organization}}. Please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`

const passwordChangedTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #1a1a2e; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; }
        .info-box { background: #e9ecef; padding: 12px; border-radius: 6px; margin: 15px 0; }
        .warning { background: #f8d7da; border: 1px solid #f5c6cb; padding: 12px; border-radius: 6px; margin-top: 20px; color: #721c24; }
        .footer { margin-top: 20px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <h2 style="margin: 0;">{{.Organization}}</h2>
    </div>
    <div class="content">
        <p>Hello,</p>
        <p>The password for user <strong>{{.UserName}}</strong> has been changed via a password reset link.</p>
        <div class="info-box">
            <strong>IP Address:</strong> {{.IPAddress}}<br>
            {{if .WhoisInfo}}<strong>Location Info:</strong> {{.WhoisInfo}}{{end}}
        </div>
        <div class="warning">
            <strong>Not you?</strong><br>
            If this password change was not authorized, please contact your system administrator immediately.
        </div>
        <div class="footer">
            <p>This is an automated message from {{.Organization}}. Please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`

const accountExpirationTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #1a1a2e; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; }
        .warning { background: #fff3cd; border: 1px solid #ffc107; padding: 12px; border-radius: 6px; margin: 15px 0; }
        .danger { background: #f8d7da; border: 1px solid #f5c6cb; padding: 12px; border-radius: 6px; margin: 15px 0; color: #721c24; }
        .info-box { background: #e9ecef; padding: 12px; border-radius: 6px; margin: 15px 0; }
        .footer { margin-top: 20px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <h2 style="margin: 0;">{{.Organization}} - Account Notice</h2>
    </div>
    <div class="content">
        <p>Hello Administrator,</p>
        {{if eq .Interval "has expired"}}
        <div class="danger">
            <strong>Account Expired</strong><br>
            The account for <strong>{{.UserName}}</strong> ({{.UserUID}}) has expired.
        </div>
        {{else}}
        <div class="warning">
            <strong>Account Expiring Soon</strong><br>
            The account for <strong>{{.UserName}}</strong> ({{.UserUID}}) will expire {{.Interval}}.
        </div>
        {{end}}
        <div class="info-box">
            <strong>Account:</strong> {{.UserUID}}<br>
            <strong>Display Name:</strong> {{.UserName}}<br>
            <strong>Expiration Date:</strong> {{.ExpirationDate}}
        </div>
        <p>Please take appropriate action to either extend or disable this account.</p>
        <div class="footer">
            <p>This is an automated message from {{.Organization}}. Please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`

const auditNotificationTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.5; color: #222; max-width: 640px; margin: 0 auto; padding: 20px; }
        .header { background: #1a1a2e; color: white; padding: 18px 22px; border-radius: 8px 8px 0 0; }
        .header h1 { margin: 0; font-size: 19px; font-weight: 600; }
        .header .sub { margin: 4px 0 0; font-size: 13px; color: #b9bdd4; }
        .content { background: #f8f9fa; padding: 22px; border-radius: 0 0 8px 8px; }
        .summary { font-size: 15px; margin: 0 0 18px; }
        table.changes { width: 100%; border-collapse: collapse; margin: 4px 0 18px; }
        table.changes th { text-align: left; font-size: 11px; text-transform: uppercase; letter-spacing: .04em; color: #6b7280; padding: 6px 8px; border-bottom: 2px solid #e5e7eb; }
        table.changes td { padding: 8px; border-bottom: 1px solid #e5e7eb; vertical-align: top; font-size: 14px; }
        table.changes td.field { font-weight: 600; width: 34%; }
        .old { color: #b91c1c; text-decoration: line-through; }
        .new { color: #047857; font-weight: 600; }
        .arrow { color: #9ca3af; padding: 0 6px; }
        .muted { color: #9ca3af; }
        .badge { display: inline-block; background: #e5e7eb; color: #374151; font-size: 12px; padding: 2px 8px; border-radius: 9999px; }
        .btn-wrap { margin: 18px 0; text-align: center; }
        .button { display: inline-block; background: #3b82f6; color: #ffffff; padding: 11px 22px; text-decoration: none; border-radius: 6px; font-weight: 600; font-size: 14px; }
        table.meta { width: 100%; border-collapse: collapse; margin: 8px 0 0; }
        table.meta td { padding: 5px 8px; border-top: 1px solid #e5e7eb; vertical-align: top; font-size: 12px; color: #6b7280; }
        table.meta td.k { width: 130px; font-weight: 600; }
        table.meta td.v { word-break: break-all; }
        .footer { margin-top: 18px; font-size: 12px; color: #6b7280; }
    </style>
</head>
<body>
    <div class="header">
        <h1>{{.Subject}}</h1>
        <p class="sub">Audit notification{{if .Organization}} &middot; {{.Organization}}{{end}}</p>
    </div>
    <div class="content">
        {{if .HasChanges}}
        {{if .IsFieldList}}
        <p class="summary">{{.Summary}}</p>
        <table class="changes">
            <tr><th>Field</th><th>Value</th></tr>
            {{range .Changes}}
            <tr>
                <td class="field">{{.Field}}</td>
                <td>{{if .Masked}}<span class="badge">set</span>{{else if .New}}<span class="new">{{.New}}</span>{{else}}<span class="muted">(empty)</span>{{end}}</td>
            </tr>
            {{end}}
        </table>
        {{else}}
        <p class="summary">The following changes were made:</p>
        <table class="changes">
            <tr><th>Field</th><th>Change</th></tr>
            {{range .Changes}}
            <tr>
                <td class="field">{{.Field}}</td>
                <td>
                    {{if .Masked}}<span class="badge">updated</span>{{else}}<span class="old">{{if .Old}}{{.Old}}{{else}}(empty){{end}}</span><span class="arrow">&rarr;</span><span class="new">{{if .New}}{{.New}}{{else}}(empty){{end}}</span>{{end}}
                </td>
            </tr>
            {{end}}
        </table>
        {{end}}
        {{else}}
        <p class="summary">{{.Summary}}</p>
        {{end}}
        <table class="meta">
            <tr><td class="k">When</td><td class="v">{{.Timestamp}}</td></tr>
            {{if .Actor}}<tr><td class="k">Performed by</td><td class="v">{{.Actor}}</td></tr>{{end}}
            <tr><td class="k">Actor DN</td><td class="v">{{.ActorDN}}</td></tr>
            {{if .ResourceDN}}<tr><td class="k">Resource DN</td><td class="v">{{.ResourceDN}}</td></tr>{{end}}
            {{if .IPAddress}}<tr><td class="k">IP address</td><td class="v">{{.IPAddress}}</td></tr>{{end}}
            {{if .UserAgent}}<tr><td class="k">User agent</td><td class="v">{{.UserAgent}}</td></tr>{{end}}
        </table>
        {{if .DetailsURL}}
        <div class="btn-wrap">
            <a href="{{.DetailsURL}}" class="button">View details</a>
        </div>
        {{end}}
        <div class="footer">
            <p>This is an automated audit notification from {{.Organization}}. Please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`

const passwordExpirationTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: #1a1a2e; color: white; padding: 20px; border-radius: 8px 8px 0 0; }
        .content { background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; }
        .warning { background: #fff3cd; border: 1px solid #ffc107; padding: 12px; border-radius: 6px; margin: 15px 0; }
        .danger { background: #f8d7da; border: 1px solid #f5c6cb; padding: 12px; border-radius: 6px; margin: 15px 0; color: #721c24; }
        .footer { margin-top: 20px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <h2 style="margin: 0;">{{.Organization}}</h2>
    </div>
    <div class="content">
        <p>Hello {{.UserName}},</p>
        {{if eq .Interval "has expired"}}
        <div class="danger">
            <strong>Password Expired</strong><br>
            Your password has expired. Please change it immediately to regain access.
        </div>
        {{else}}
        <div class="warning">
            <strong>Password Expiring Soon</strong><br>
            Your password will expire {{.Interval}}.
        </div>
        {{end}}
        <p><strong>Expiration Date:</strong> {{.ExpirationDate}}</p>
        <p>Please change your password before the expiration date to avoid any disruption to your access.</p>
        <div class="footer">
            <p>This is an automated message from {{.Organization}}. Please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`
