package wireguard

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"net"
	"strconv"
	"testing"

	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/transport/internet"
)

type startupOutboundTag struct{ outbound.Handler }

func (startupOutboundTag) Tag() string { return "" }

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
			instance, err := core.New(&core.Config{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = instance.Close() })
			ctx := context.WithValue(context.Background(), core.XrayKey(1), instance)
			ctx = session.ContextWithFullHandler(ctx, startupOutboundTag{})
			ctx = session.ContextWithStreamSettings(ctx, &internet.MemoryStreamConfig{})
			handler, err := NewClient(ctx, &DeviceConfig{
				SecretKey: hex.EncodeToString(private.Bytes()),
				Endpoint:  []string{"10.253.0.1"}, Mtu: 1380, NoKernelTun: true,
				Peers: []*PeerConfig{{
					PublicKey: hex.EncodeToString(peer.PublicKey().Bytes()),
					Endpoint:  listener.LocalAddr().String(), AllowedIps: []string{"0.0.0.0/0"},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = handler.Close() })
			if err := handler.up(context.Background()); err != nil {
				t.Fatal(err)
			}
			first := handler.dev
			if first == nil {
				t.Fatal("初始化没有创建 WireGuard 设备")
			}
			if err := first.Down(); err != nil {
				t.Fatal(err)
			}
			if err := handler.up(context.Background()); err != nil {
				t.Fatal(err)
			}
			if handler.dev != first {
				t.Fatal("重复初始化替换了已有设备")
			}
			if err := handler.Close(); err != nil {
				t.Fatal(err)
			}
			if err := handler.up(context.Background()); err == nil {
				t.Fatal("已经关闭的设备被重新初始化")
			}
		})
	}
}
