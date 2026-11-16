package dispatcher

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/xtls/xray-core/app/router"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/geodata"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	routing_session "github.com/xtls/xray-core/features/routing/session"
	"github.com/xtls/xray-core/proxy/blackhole"
	"github.com/xtls/xray-core/transport"
)

const testDHTPing = "d1:ad2:id20:01234567890123456789e1:q4:ping1:t2:aa1:y1:qe"

type dhtTestReader struct {
	packets     buf.MultiBuffer
	interrupted bool
}

func (r *dhtTestReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	if r.packets == nil {
		return nil, io.EOF
	}
	packets := r.packets
	r.packets = nil
	return packets, nil
}
func (r *dhtTestReader) ReadMultiBufferTimeout(time.Duration) (buf.MultiBuffer, error) {
	return r.ReadMultiBuffer()
}
func (r *dhtTestReader) Interrupt() { r.interrupted = true }

type dhtTestHandler struct {
	outbound.Handler
	observer func(net.Destination)
}

func (dhtTestHandler) ProxySettings() *serial.TypedMessage {
	return serial.ToTypedMessage(&blackhole.Config{})
}

func (h dhtTestHandler) BlockedDHTPacket(_ context.Context, destination net.Destination) {
	if h.observer != nil {
		h.observer(destination)
	}
}

type dhtTestManager struct {
	outbound.Manager
	observer func(net.Destination)
}

func (m dhtTestManager) GetHandler(tag string) outbound.Handler {
	if tag == "block" {
		return dhtTestHandler{observer: m.observer}
	}
	return nil
}

func dhtTestPacket(payload string, destination net.Destination) *buf.Buffer {
	packet := buf.New()
	_, _ = packet.Write([]byte(payload))
	packet.UDP = &destination
	return packet
}

func TestDHTReaderPreservesPacketsAndTargets(t *testing.T) {
	destination := net.UDPDestination(net.LocalHostIP, 6881)
	other := net.UDPDestination(net.LocalHostIP, 6882)
	config := &router.Config{Rule: []*router.RoutingRule{{
		TargetTag: &router.RoutingRule_Tag{Tag: "block"}, Protocol: []string{"bittorrent"},
		PortList: &net.PortList{Range: []*net.PortRange{{From: 6881, To: 6881}}},
		Networks: []net.Network{net.Network_UDP}, InboundTag: []string{"user-in"},
	}}}
	observed := 0
	manager := dhtTestManager{observer: func(target net.Destination) {
		observed++
		if target != destination {
			t.Fatalf("incorrect block target: %v", target)
		}
	}}
	picker := new(router.Router)
	if err := picker.Init(context.Background(), config, nil, manager, nil); err != nil {
		t.Fatal(err)
	}
	if !picker.HasDHTBlockRule() {
		t.Fatal("bound BT block was not detected")
	}
	ctx := session.ContextWithInbound(context.Background(), &session.Inbound{Tag: "user-in"})
	ctx = session.ContextWithOutbounds(ctx, []*session.Outbound{{Target: destination}})
	ctx = session.ContextWithContent(ctx, &session.Content{Protocol: "dns"})
	packets := buf.MultiBuffer{
		dhtTestPacket("normal", destination), dhtTestPacket(testDHTPing, destination),
		dhtTestPacket("after", destination), dhtTestPacket(testDHTPing, other),
		dhtTestPacket(testDHTPing+"tail", destination), dhtTestPacket(testDHTPing, destination),
	}
	underlying := &dhtTestReader{packets: packets}
	reader := &dhtReader{
		Reader: underlying, ctx: ctx, dispatcher: &DefaultDispatcher{ohm: manager},
		picker: picker, routing: routing_session.AsRoutingContext(ctx), destination: destination,
	}
	kept, err := reader.ReadMultiBuffer()
	defer buf.ReleaseMulti(kept)
	if err != nil || len(kept) != 4 {
		t.Fatalf("packets were lost or blocked data forwarded: len=%d err=%v", len(kept), err)
	}
	for i, want := range []string{"normal", "after", testDHTPing, testDHTPing + "tail"} {
		if string(kept[i].Bytes()) != want {
			t.Fatalf("packet order/data changed at %d", i)
		}
	}
	if *kept[2].UDP != other || observed != 1 {
		t.Fatalf("target or deduplication changed: target=%v logs=%d", kept[2].UDP, observed)
	}
	if session.ContentFromContext(ctx).Protocol != "dns" {
		t.Fatal("shared session protocol was changed")
	}
	if _, err := reader.ReadMultiBuffer(); err != io.EOF {
		t.Fatalf("reader error lost: %v", err)
	}
	reader.Interrupt()
	if !underlying.interrupted {
		t.Fatal("underlying transport was not interrupted")
	}
}

func TestCachedReaderKeepsDatagramBoundaries(t *testing.T) {
	destination := net.UDPDestination(net.LocalHostIP, 6881)
	underlying := &dhtTestReader{packets: buf.MultiBuffer{
		dhtTestPacket(testDHTPing, destination), dhtTestPacket(testDHTPing, destination),
	}}
	reader := &cachedReader{reader: underlying}
	payload := buf.New()
	defer payload.Release()
	if err := reader.Cache(payload, time.Second); err != nil {
		t.Fatal(err)
	}
	probe := &Sniffer{sniffer: []protocolSnifferWithMetadata{{
		network: net.Network_UDP, packetOnly: true,
		protocolSniffer: func(_ context.Context, packet []byte) (SniffResult, error) {
			if string(packet) != testDHTPing {
				t.Fatal("first packet boundary lost")
			}
			return nil, io.EOF
		},
	}}}
	_, _ = reader.SniffPacket(context.Background(), probe, payload.Bytes(), net.Network_UDP)
	if !reader.FirstPacketIsDHT() || string(payload.Bytes()) != testDHTPing+testDHTPing {
		t.Fatal("DHT packet boundary or accumulated QUIC payload was changed")
	}
	packets, err := reader.ReadMultiBuffer()
	defer buf.ReleaseMulti(packets)
	if err != nil || len(packets) != 2 {
		t.Fatalf("cached packets were consumed by sniffing: %d %v", len(packets), err)
	}
}

func TestSnifferDoesNotJoinDHTFragments(t *testing.T) {
	ctx := context.WithValue(context.Background(), core.XrayKey(1), new(core.Instance))
	first, second := testDHTPing[:25], testDHTPing[25:]
	result, err := NewSniffer(ctx).SniffPacket(ctx, []byte(first+second), []byte(first), net.Network_UDP)
	if err == nil || result != nil {
		t.Fatal("two non-DHT datagrams were joined into a DHT match")
	}
	result, err = NewSniffer(ctx).SniffPacket(ctx, []byte(testDHTPing+testDHTPing), []byte(testDHTPing), net.Network_UDP)
	if err != nil || result.Protocol() != "bittorrent" {
		t.Fatalf("complete first datagram was not recognized: %v", err)
	}
	seen := ""
	sniffer := &Sniffer{sniffer: []protocolSnifferWithMetadata{{
		network: net.Network_UDP,
		protocolSniffer: func(_ context.Context, payload []byte) (SniffResult, error) {
			seen = string(payload)
			return nil, io.EOF
		},
	}}}
	_, _ = sniffer.SniffPacket(ctx, []byte("accumulated"), []byte("first"), net.Network_UDP)
	if seen != "accumulated" {
		t.Fatal("non-KRPC UDP sniffer lost its accumulated payload")
	}
}

type dhtCaptureHandler struct {
	outbound.Handler
	link *transport.Link
}

func (*dhtCaptureHandler) Tag() string                                        { return "direct" }
func (*dhtCaptureHandler) ProxySettings() *serial.TypedMessage                { return nil }
func (h *dhtCaptureHandler) Dispatch(_ context.Context, link *transport.Link) { h.link = link }

type dhtCaptureManager struct {
	dhtTestManager
	direct *dhtCaptureHandler
}

func (m dhtCaptureManager) GetHandler(tag string) outbound.Handler {
	if tag == "direct" {
		return m.direct
	}
	return m.dhtTestManager.GetHandler(tag)
}
func (m dhtCaptureManager) GetDefaultHandler() outbound.Handler { return m.direct }

func TestDHTRouteOnlyPreservesActualTargetIP(t *testing.T) {
	destination := net.UDPDestination(net.ParseAddress("192.0.2.1"), 6881)
	sniffed := net.UDPDestination(net.DomainAddress("dht.example"), 6881)
	ipCondition := []*geodata.IPRule{{Value: &geodata.IPRule_Custom{Custom: &geodata.CIDRRule{Cidr: &geodata.CIDR{Ip: []byte{192, 0, 2, 1}, Prefix: 32}}}}}
	for _, mode := range []string{"ip_block", "earlier_ip_allow"} {
		t.Run(mode, func(t *testing.T) {
			block := &router.RoutingRule{TargetTag: &router.RoutingRule_Tag{Tag: "block"}, Protocol: []string{"bittorrent"}}
			rules := []*router.RoutingRule{block}
			if mode == "ip_block" {
				block.Ip = ipCondition
			} else {
				rules = append([]*router.RoutingRule{{TargetTag: &router.RoutingRule_Tag{Tag: "direct"}, Ip: ipCondition}}, rules...)
			}
			handler := &dhtCaptureHandler{}
			manager := dhtCaptureManager{direct: handler}
			picker := new(router.Router)
			if err := picker.Init(context.Background(), &router.Config{Rule: rules}, nil, manager, nil); err != nil {
				t.Fatal(err)
			}
			ctx := session.ContextWithOutbounds(context.Background(), []*session.Outbound{{Target: destination, RouteTarget: sniffed}})
			ctx = session.ContextWithContent(ctx, &session.Content{Protocol: "quic"})
			packet := buf.FromBytes([]byte(testDHTPing)) // Fixed-target UDP has no per-packet address.
			dispatcher := &DefaultDispatcher{ohm: manager, router: picker}
			dispatcher.routedDispatch(ctx, &transport.Link{Reader: &dhtTestReader{packets: buf.MultiBuffer{packet}}}, sniffed)
			if handler.link == nil {
				t.Fatal("ordinary UDP was not routed")
			}
			kept, err := handler.link.Reader.ReadMultiBuffer()
			defer buf.ReleaseMulti(kept)
			if mode == "ip_block" {
				if len(kept) != 0 || err != io.EOF {
					t.Fatalf("IP block missed: %d %v", len(kept), err)
				}
			} else if len(kept) != 1 || err != nil {
				t.Fatalf("earlier IP allow ignored: %d %v", len(kept), err)
			}
			if session.ContentFromContext(ctx).Protocol != "quic" {
				t.Fatal("shared protocol changed")
			}
		})
	}
}
