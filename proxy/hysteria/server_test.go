package hysteria

import (
	stdnet "net"
	"testing"

	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/transport/internet/stat"
)

type authenticatedTestConnection struct {
	stdnet.Conn
	user *protocol.MemoryUser
}

func (c *authenticatedTestConnection) User() *protocol.MemoryUser {
	return c.user
}

func TestAuthenticatedUserUnwrapsStatsConnection(t *testing.T) {
	client, server := stdnet.Pipe()
	defer client.Close()
	defer server.Close()

	want := &protocol.MemoryUser{Email: "user@42", Level: 7}
	conn := &authenticatedTestConnection{Conn: server, user: want}
	wrapped := &stat.CounterConnection{Connection: conn}

	if got := authenticatedUser(wrapped); got != want {
		t.Fatalf("authenticatedUser() = %v, want %v", got, want)
	}
}

func TestAuthenticatedUserReturnsNilWithoutUserMetadata(t *testing.T) {
	client, server := stdnet.Pipe()
	defer client.Close()
	defer server.Close()

	wrapped := &stat.CounterConnection{Connection: server}
	if got := authenticatedUser(wrapped); got != nil {
		t.Fatalf("authenticatedUser() = %v, want nil", got)
	}
}
