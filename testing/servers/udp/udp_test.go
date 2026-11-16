package udp

import (
	"bytes"
	"testing"
	"time"

	"github.com/xtls/xray-core/common/net"
)

func TestServerEchoAndClose(t *testing.T) {
	server := &Server{MsgProcessor: func(msg []byte) []byte { return msg }}
	destination, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP: destination.Address.IP(), Port: int(destination.Port),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := []byte("udp-server-close-regression")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload)+1)
	n, err := conn.Read(response)
	if err != nil || !bytes.Equal(response[:n], payload) {
		t.Fatalf("echo response = %q, error = %v", response[:n], err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServerCloseBeforeReadLoopStarts(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: []byte{127, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{conn: conn, MsgProcessor: func(msg []byte) []byte { return msg }}
	server.accepting.Store(true)
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		server.handleConnection(conn)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closed UDP server read loop did not exit")
	}
}
