package wireguard

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/internet"
	"golang.zx2c4.com/wireguard/tun"
)

type startupPacket struct {
	destination net.Destination
	payload     string
}

type startupDispatcher struct {
	routing.Dispatcher
	received chan startupPacket
}

func (d *startupDispatcher) DispatchLink(_ context.Context, destination net.Destination, link *transport.Link) error {
	data, err := link.Reader.ReadMultiBuffer()
	if err != nil {
		return err
	}
	defer buf.ReleaseMulti(data)
	d.received <- startupPacket{destination: destination, payload: data.String()}
	return nil
}

type startupPacketTun struct {
	tun.Device
	once   sync.Once
	packet []byte
}

func (t *startupPacketTun) MTU() (int, error) {
	// 收包设备第一次访问 TUN 时送入已到达的数据，固定复现启动先后顺序。
	t.once.Do(func() { _, _ = t.Device.Write([][]byte{t.packet}, 0) })
	return t.Device.MTU()
}

func TestServerForwardsPacketAtDeviceStartup(t *testing.T) {
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := "startup-packet"
	packet := make([]byte, 20+8+len(payload))
	packet[0], packet[8], packet[9] = 0x45, 64, 17
	binary.BigEndian.PutUint16(packet[2:], uint16(len(packet)))
	copy(packet[12:16], []byte{10, 253, 0, 1})
	copy(packet[16:20], []byte{10, 253, 0, 2})
	binary.BigEndian.PutUint16(packet[10:], ^onesSum(packet[:20], 0))
	binary.BigEndian.PutUint16(packet[20:], 54321)
	binary.BigEndian.PutUint16(packet[22:], 12345)
	binary.BigEndian.PutUint16(packet[24:], uint16(8+len(payload)))
	copy(packet[28:], payload)
	account := &MemoryAccount{AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.253.0.1/32")}}
	copy(account.Pub[:], peer.PublicKey().Bytes())
	users := &sync.Map{}
	users.Store(account.Pub, &protocol.MemoryUser{Account: account, Email: "startup-test"})
	dispatcher := &startupDispatcher{received: make(chan startupPacket, 2)}
	server := &Server{
		conf: &DeviceConfig{SecretKey: hex.EncodeToString(private.Bytes())},
		ctx:  context.Background(), dispatcher: dispatcher,
		src: net.UDPDestination(net.LocalHostIP, 0), streamSettings: &internet.MemoryStreamConfig{},
		users: users,
	}
	tunDevice, _, err := CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.253.0.2")}, nil, 1380, server)
	if err != nil {
		t.Fatal(err)
	}
	server.tun = &startupPacketTun{Device: tunDevice, packet: packet}
	t.Cleanup(func() { _ = server.Close() })
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-dispatcher.received:
		if got.payload != payload || got.destination != net.UDPDestination(net.IPAddress([]byte{10, 253, 0, 2}), 12345) {
			t.Fatalf("启动时的数据转发错误：%+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("收包设备启动时，转发处理尚未就绪，首包丢失")
	}
	first := server.dev
	if err := server.Start(); err != nil || server.dev != first {
		t.Fatalf("重复启动未保留已有设备：%v", err)
	}
}
