//go:build linux

package internet

import (
	"context"
	"net"
	"syscall"
	"testing"

	"github.com/pires/go-proxyproto"
	"golang.org/x/sys/unix"
)

func TestDefaultListenerDisablesMPTCP(t *testing.T) {
	// Go's process default must not override the core's default TCP listener.
	t.Setenv("GODEBUG", "multipathtcp=1")
	for _, tc := range []struct {
		name    string
		sockopt *SocketConfig
	}{
		{name: "no_socket_config"},
		{name: "empty_socket_config", sockopt: &SocketConfig{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dl := DefaultListener{}
			listener, err := dl.Listen(context.Background(), &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, tc.sockopt)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			socketListener := listener
			if proxyListener, ok := listener.(*proxyproto.Listener); ok {
				socketListener = proxyListener.Listener
			}
			conn, ok := socketListener.(syscall.Conn)
			if !ok {
				t.Fatalf("listener %T does not expose its socket", socketListener)
			}
			raw, err := conn.SyscallConn()
			if err != nil {
				t.Fatal(err)
			}
			var protocol int
			var socketErr error
			if err := raw.Control(func(fd uintptr) {
				protocol, socketErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PROTOCOL)
			}); err != nil {
				t.Fatal(err)
			}
			if socketErr != nil {
				t.Fatal(socketErr)
			}
			if protocol != unix.IPPROTO_TCP {
				t.Fatalf("listener protocol = %d, want TCP (%d)", protocol, unix.IPPROTO_TCP)
			}
		})
	}
}
