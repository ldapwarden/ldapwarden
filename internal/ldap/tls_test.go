package ldap

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// testPKI is a private CA with a server certificate for 127.0.0.1 and a client
// certificate, written to PEM files the way an operator would mount them.
type testPKI struct {
	caFile, certFile, keyFile string // client side, as the app is configured
	server                    tls.Certificate
	pool                      *x509.CertPool
}

func newTestPKI(t *testing.T) testPKI {
	t.Helper()
	dir := t.TempDir()

	caKey := newKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ldapwarden test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	issue := func(serial int64, cn string, usage x509.ExtKeyUsage) ([]byte, *ecdsa.PrivateKey) {
		key := newKey(t)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return der, key
	}

	serverDER, serverKey := issue(2, "directory", x509.ExtKeyUsageServerAuth)
	clientDER, clientKey := issue(3, "ldapwarden-client", x509.ExtKeyUsageClientAuth)

	pki := testPKI{
		caFile:   writePEM(t, dir, "ca.pem", "CERTIFICATE", caDER),
		certFile: writePEM(t, dir, "client.pem", "CERTIFICATE", clientDER),
		keyFile:  writePEM(t, dir, "client.key", "EC PRIVATE KEY", marshalKey(t, clientKey)),
		server:   tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey},
		pool:     x509.NewCertPool(),
	}
	pki.pool.AddCert(caCert)
	return pki
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func marshalKey(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func writePEM(t *testing.T, dir, name, blockType string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// serverConfig is the directory's side. With requireClient it demands a client
// certificate signed by the test CA, as olcTLSVerifyClient: demand does.
func (p testPKI) serverConfig(requireClient bool) *tls.Config {
	cfg := &tls.Config{Certificates: []tls.Certificate{p.server}}
	if requireClient {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = p.pool
	}
	return cfg
}

// handshakes runs a listener that completes the TLS handshake on each
// connection — after answering a StartTLS request first, when startTLS is set —
// and reports the outcome: the client certificate's CN, or the error.
func handshakes(t *testing.T, serverTLS *tls.Config, startTLS bool) (addr string, results <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	out := make(chan string, 4)
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = raw.Close() }()
				_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
				if startTLS {
					if err := answerStartTLS(raw); err != nil {
						out <- "starttls: " + err.Error()
						return
					}
				}
				conn := tls.Server(raw, serverTLS)
				if err := conn.Handshake(); err != nil {
					out <- "handshake: " + err.Error()
					return
				}
				cn := ""
				if peers := conn.ConnectionState().PeerCertificates; len(peers) > 0 {
					cn = peers[0].Subject.CommonName
				}
				out <- "ok " + cn
				// Hold the connection until the client hangs up.
				_, _ = conn.Read(make([]byte, 1))
			}()
		}
	}()
	return ln.Addr().String(), out
}

// answerStartTLS reads the client's StartTLS extended request and answers
// success, which is all a server says before the handshake.
func answerStartTLS(conn net.Conn) error {
	request, err := ber.ReadPacket(conn)
	if err != nil {
		return err
	}
	messageID := request.Children[0].Value.(int64)

	response := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "LDAP Response")
	response.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, messageID, "MessageID"))
	extended := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationExtendedResponse, nil, "Extended Response")
	extended.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, int64(ldap.LDAPResultSuccess), "resultCode"))
	extended.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "matchedDN"))
	extended.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "diagnosticMessage"))
	response.AppendChild(extended)
	_, err = conn.Write(response.Bytes())
	return err
}

func awaitResult(t *testing.T, results <-chan string) string {
	t.Helper()
	select {
	case r := <-results:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the server saw no handshake")
		return ""
	}
}

// TestDialLDAPSWithPrivateCA is the case LDAP_TLS_CA_FILE exists for: a
// directory whose certificate a private CA signed. The only way to reach one
// used to be LDAP_TLS_SKIP_VERIFY.
func TestDialLDAPSWithPrivateCA(t *testing.T) {
	pki := newTestPKI(t)
	addr, results := handshakes(t, pki.serverConfig(false), false)

	client := NewClient(config.LDAPConfig{
		URL: "ldaps://" + addr, TLSMode: "ssl", TLSCAFile: pki.caFile, Timeout: 5 * time.Second,
	})
	conn, err := client.dial()
	if err != nil {
		t.Fatalf("dial with the CA file: %v", err)
	}
	_ = conn.Close()
	if got := awaitResult(t, results); got != "ok " {
		t.Errorf("server saw %q", got)
	}
}

// TestDialLDAPSWithoutCAFailsAndSaysWhy pins the error an operator gets when
// the certificate does not verify: which settings were in play, since the TLS
// error itself names neither.
func TestDialLDAPSWithoutCAFailsAndSaysWhy(t *testing.T) {
	pki := newTestPKI(t)
	addr, _ := handshakes(t, pki.serverConfig(false), false)

	client := NewClient(config.LDAPConfig{URL: "ldaps://" + addr, TLSMode: "ssl", Timeout: 5 * time.Second})
	_, err := client.dial()
	if err == nil {
		t.Fatal("a certificate from an unknown CA was accepted")
	}
	if !strings.Contains(err.Error(), "[TLS mode ssl, system CAs]") {
		t.Errorf("error %q does not say which TLS settings were used", err)
	}
}

// TestDialPresentsClientCertificate covers mutual TLS: a directory configured
// with olcTLSVerifyClient demand only lets in a client with a certificate.
func TestDialPresentsClientCertificate(t *testing.T) {
	pki := newTestPKI(t)
	addr, results := handshakes(t, pki.serverConfig(true), false)

	client := NewClient(config.LDAPConfig{
		URL: "ldaps://" + addr, TLSMode: "ssl", TLSCAFile: pki.caFile,
		TLSCertFile: pki.certFile, TLSKeyFile: pki.keyFile, Timeout: 5 * time.Second,
	})
	conn, err := client.dial()
	if err != nil {
		t.Fatalf("dial with a client certificate: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if got := awaitResult(t, results); got != "ok ldapwarden-client" {
		t.Errorf("server saw %q, want the client certificate", got)
	}
}

// TestDialStartTLSVerifies is the regression test for StartTLS with
// verification on, which could never connect: go-ldap hands the TLS config to
// tls.Client as is, and with no ServerName Go refuses to verify anything.
func TestDialStartTLSVerifies(t *testing.T) {
	pki := newTestPKI(t)
	addr, results := handshakes(t, pki.serverConfig(false), true)

	client := NewClient(config.LDAPConfig{
		URL: "ldap://" + addr, TLSMode: "starttls", TLSCAFile: pki.caFile, Timeout: 5 * time.Second,
	})
	conn, err := client.dial()
	if err != nil {
		t.Fatalf("StartTLS with verification: %v", err)
	}
	_ = conn.Close()
	if got := awaitResult(t, results); got != "ok " {
		t.Errorf("server saw %q", got)
	}
}

// TestDialRefusesSSLOnPlainURL: go-ldap only encrypts the ldaps:// scheme, so
// this combination used to connect in clear while the setting said otherwise.
func TestDialRefusesSSLOnPlainURL(t *testing.T) {
	client := NewClient(config.LDAPConfig{URL: "ldap://127.0.0.1:1", TLSMode: "ssl", Timeout: time.Second})
	if _, err := client.dial(); err == nil || !strings.Contains(err.Error(), "ldaps://") {
		t.Fatalf("dial = %v, want a refusal naming ldaps://", err)
	}
}

func TestTLSConfigRejectsUnusableFiles(t *testing.T) {
	dir := t.TempDir()
	notPEM := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := TLSConfig(config.LDAPConfig{URL: "ldaps://dir", TLSCAFile: notPEM}); err == nil ||
		!strings.Contains(err.Error(), "no PEM certificate") {
		t.Errorf("CA file without a certificate: err = %v", err)
	}
	if _, err := TLSConfig(config.LDAPConfig{URL: "ldaps://dir", TLSCAFile: filepath.Join(dir, "absent")}); err == nil {
		t.Error("missing CA file accepted")
	}
	if _, err := TLSConfig(config.LDAPConfig{URL: "ldaps://dir", TLSCertFile: notPEM, TLSKeyFile: notPEM}); err == nil {
		t.Error("unusable client certificate accepted")
	}

	cfg, err := TLSConfig(config.LDAPConfig{URL: "ldaps://directory.example.org:636"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "directory.example.org" {
		t.Errorf("ServerName = %q, want the URL's host", cfg.ServerName)
	}
}
