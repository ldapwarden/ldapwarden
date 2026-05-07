package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/ldapwarden/ldapwarden/internal/config"
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
}

func NewMailer(cfg *config.MailConfig, organization string) *Mailer {
	return &Mailer{
		config:       cfg,
		organization: organization,
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

// SendAuditNotification sends a per-change audit email to each recipient.
// Args are primitives to keep this package free of an internal/audit import.
// Each modification action recorded in the audit log produces one call.
func (m *Mailer) SendAuditNotification(
	recipients []string,
	timestamp time.Time,
	actorUID, actorDN string,
	action, resourceType, resourceDN string,
	details map[string]interface{},
	ipAddress, userAgent string,
) error {
	if len(recipients) == 0 {
		return nil
	}

	subject := fmt.Sprintf("[%s] %s by %s", m.organization, action, displayActor(actorUID, actorDN))

	detailsJSON := ""
	if len(details) > 0 {
		if b, err := json.MarshalIndent(details, "", "  "); err == nil {
			detailsJSON = string(b)
		}
	}

	data := map[string]string{
		"Organization": m.organization,
		"Timestamp":    timestamp.UTC().Format("2006-01-02 15:04:05 MST"),
		"Actor":        displayActor(actorUID, actorDN),
		"ActorDN":      actorDN,
		"Action":       action,
		"ResourceType": resourceType,
		"ResourceDN":   resourceDN,
		"Details":      detailsJSON,
		"IPAddress":    ipAddress,
		"UserAgent":    userAgent,
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

func displayActor(uid, dn string) string {
	if uid != "" {
		return uid
	}
	if dn != "" {
		return dn
	}
	return "unknown"
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
	for _, h := range []string{m.config.From, to, subject} {
		if err := validateHeaderValue(h); err != nil {
			return nil, err
		}
	}
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", m.config.From))
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

	// Check if STARTTLS is supported and upgrade
	encrypted := false
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: m.config.Host,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
		encrypted = true
	}

	return m.sendWithClient(client, to, msg, encrypted)
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

func (m *Mailer) renderTemplate(tmpl string, data map[string]string) (string, error) {
	t, err := template.New("email").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
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
        .header { background: #1a1a2e; color: white; padding: 16px 20px; border-radius: 8px 8px 0 0; }
        .header h2 { margin: 0; font-size: 18px; }
        .content { background: #f8f9fa; padding: 20px; border-radius: 0 0 8px 8px; }
        table.meta { width: 100%; border-collapse: collapse; margin: 8px 0 16px; }
        table.meta td { padding: 6px 8px; border-bottom: 1px solid #e5e7eb; vertical-align: top; font-size: 14px; }
        table.meta td.k { width: 140px; color: #6b7280; font-weight: 600; }
        table.meta td.v { word-break: break-all; }
        pre { background: #1f2937; color: #e5e7eb; padding: 12px; border-radius: 6px; font-size: 13px; overflow-x: auto; white-space: pre-wrap; word-break: break-word; }
        .footer { margin-top: 16px; font-size: 12px; color: #6b7280; }
    </style>
</head>
<body>
    <div class="header">
        <h2>{{.Organization}} — Audit notification</h2>
    </div>
    <div class="content">
        <p>The following change was just recorded in the audit log:</p>
        <table class="meta">
            <tr><td class="k">When</td><td class="v">{{.Timestamp}}</td></tr>
            <tr><td class="k">Actor</td><td class="v">{{.Actor}}</td></tr>
            <tr><td class="k">Actor DN</td><td class="v">{{.ActorDN}}</td></tr>
            <tr><td class="k">Action</td><td class="v"><code>{{.Action}}</code></td></tr>
            <tr><td class="k">Resource type</td><td class="v">{{.ResourceType}}</td></tr>
            {{if .ResourceDN}}<tr><td class="k">Resource DN</td><td class="v">{{.ResourceDN}}</td></tr>{{end}}
            {{if .IPAddress}}<tr><td class="k">IP address</td><td class="v">{{.IPAddress}}</td></tr>{{end}}
            {{if .UserAgent}}<tr><td class="k">User agent</td><td class="v">{{.UserAgent}}</td></tr>{{end}}
        </table>
        {{if .Details}}
        <p style="margin-bottom: 6px;"><strong>Details</strong></p>
        <pre>{{.Details}}</pre>
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
