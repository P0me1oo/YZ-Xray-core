package wireguard

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/netip"
	"strconv"
	"testing"

	"github.com/xtls/xray-core/transport/internet"
)

// 用户态设备创建时已经带有启用事件，初始化必须能安全接住这次提前启动。
func TestClientInitWithPendingUpEvent(t *testing.T) {
	for i := range 32 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			private, err := ecdh.X25519().GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			peer, err := ecdh.X25519().GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			tunDevice, tnet, _, err := CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.253.0.1")}, nil, 1380, true)
			if err != nil {
				t.Fatal(err)
			}
			handler := &Handler{
				conf: &DeviceConfig{
					SecretKey: hex.EncodeToString(private.Bytes()),
					Peers: []*PeerConfig{{
						PublicKey:  hex.EncodeToString(peer.PublicKey().Bytes()),
						Endpoint:   listener.LocalAddr().String(),
						AllowedIps: []string{"0.0.0.0/0"},
					}},
				},
				streamSettings: &internet.MemoryStreamConfig{},
				tun:            tunDevice,
				tnet:           tnet,
			}
			t.Cleanup(func() { _ = handler.Close() })
			if err := handler.init(context.Background()); err != nil {
				t.Fatal(err)
			}
			first := handler.dev
			if first == nil {
				t.Fatal("初始化没有创建 WireGuard 设备")
			}
			if err := first.Down(); err != nil {
				t.Fatal(err)
			}
			if err := handler.init(context.Background()); err != nil {
				t.Fatal(err)
			}
			if handler.dev != first {
				t.Fatal("重复初始化替换了已有设备")
			}
			if err := handler.Close(); err != nil {
				t.Fatal(err)
			}
			if err := handler.init(context.Background()); err == nil {
				t.Fatal("已经关闭的设备被重新初始化")
			}
		})
	}
}
