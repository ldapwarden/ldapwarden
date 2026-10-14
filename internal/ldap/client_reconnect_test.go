package ldap

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"

	"github.com/ldapwarden/ldapwarden/internal/config"
)

// answerBind reads one request, a bind, and answers success.
func answerBind(conn net.Conn) error {
	request, err := ber.ReadPacket(conn)
	if err != nil {
		return err
	}
	messageID := request.Children[0].Value.(int64)
	response := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "LDAP Response")
	response.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, messageID, "MessageID"))
	bind := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationBindResponse, nil, "Bind Response")
	bind.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, int64(ldap.LDAPResultSuccess), "resultCode"))
	bind.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "matchedDN"))
	bind.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "diagnosticMessage"))
	response.AppendChild(bind)
	_, err = conn.Write(response.Bytes())
	return err
}

// TestConnectReplacesAClosedConnection: the shared connection is reused without
// a liveness check (see connect), so a server that closes it — a restart — has
// to be noticed some other way. go-ldap marks the connection closing when the
// socket fails; the next request must then dial a new one rather than fail.
func TestConnectReplacesAClosedConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var accepted atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			n := accepted.Add(1)
			go func() {
				defer func() { _ = conn.Close() }()
				if answerBind(conn) != nil || n == 1 {
					// The first connection is dropped right after the bind, as
					// a restarting server would.
					return
				}
				_, _ = conn.Read(make([]byte, 1))
			}()
		}
	}()

	client := NewClient(config.LDAPConfig{URL: "ldap://" + ln.Addr().String(), BindDN: "cn=admin", BindPass: "x", Timeout: 2 * time.Second})
	t.Cleanup(client.Close)

	first, err := client.connect()
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !first.IsClosing() {
		if time.Now().After(deadline) {
			t.Fatal("go-ldap never noticed the server closed the connection")
		}
		time.Sleep(10 * time.Millisecond)
	}

	second, err := client.connect()
	if err != nil {
		t.Fatalf("connect after the server closed the connection: %v", err)
	}
	if second == first || accepted.Load() != 2 {
		t.Errorf("the closed connection was reused (connections accepted: %d)", accepted.Load())
	}
}
