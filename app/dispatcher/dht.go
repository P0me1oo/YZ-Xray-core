package dispatcher

import (
	"context"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/dht"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/proxy/blackhole"
)

type dhtRoutePicker interface {
	PickDHTRoute(routing.Context) (routing.Route, error)
	HasDHTBlockRule() bool
}

type dhtBlockObserver interface {
	BlockedDHTPacket(context.Context, net.Destination)
}

// No UnwrapReader: packet inspection must survive optimized copying and XUDP.
type dhtReader struct {
	buf.Reader
	ctx         context.Context
	dispatcher  *DefaultDispatcher
	picker      dhtRoutePicker
	routing     routing.Context
	destination net.Destination
	reported    map[string]struct{}
}

func (r *dhtReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	for {
		buffers, err := r.Reader.ReadMultiBuffer()
		kept := buffers[:0]
		for _, packet := range buffers {
			if packet == nil {
				continue
			}
			destination := r.destination
			if packet.UDP != nil {
				destination = *packet.UDP
			}
			if r.block(packet.Bytes(), destination) {
				packet.Release()
			} else {
				kept = append(kept, packet)
			}
		}
		clear(buffers[len(kept):])
		if len(kept) > 0 || err != nil {
			return kept, err
		}
	}
}

func (r *dhtReader) Interrupt() {
	// Preserve the transport's native interruption and cached-buffer cleanup.
	if reader, ok := r.Reader.(interface{ Interrupt() }); ok {
		reader.Interrupt()
	}
}

func (r *dhtReader) block(packet []byte, destination net.Destination) bool {
	if !dht.IsMessage(packet) {
		return false
	}
	route, err := r.picker.PickDHTRoute(dhtContext{Context: r.routing, destination: destination})
	if err != nil {
		return false
	}
	tag := route.GetOutboundTag()
	handler := r.dispatcher.ohm.GetHandler(tag)
	if !isBlackhole(handler) {
		return false
	}
	key := destination.String() + "\x00" + tag
	if r.reported == nil {
		r.reported = make(map[string]struct{})
	}
	if _, exists := r.reported[key]; !exists && len(r.reported) < 256 {
		r.reported[key] = struct{}{}
		if observer, ok := handler.(dhtBlockObserver); ok {
			observer.BlockedDHTPacket(r.ctx, destination)
		}
	}
	return true
}

func isBlackhole(handler outbound.Handler) bool {
	return handler != nil && handler.ProxySettings() != nil &&
		handler.ProxySettings().Type == serial.GetMessageType(&blackhole.Config{})
}

// Overrides are local to the datagram; shared mux session metadata is untouched.
type dhtContext struct {
	routing.Context
	destination net.Destination
}

func (c dhtContext) GetProtocol() string     { return "bittorrent" }
func (c dhtContext) GetNetwork() net.Network { return net.Network_UDP }
func (c dhtContext) GetTargetPort() net.Port { return c.destination.Port }
func (c dhtContext) GetTargetDomain() string {
	if c.destination.Address.Family().IsDomain() {
		return c.destination.Address.Domain()
	}
	if c.GetTargetPort() == c.Context.GetTargetPort() {
		original := c.Context.GetTargetIPs()
		for _, ip := range original {
			if ip.Equal(c.destination.Address.IP()) {
				return c.Context.GetTargetDomain()
			}
		}
	}
	return ""
}
func (c dhtContext) GetTargetIPs() []net.IP {
	if c.destination.Address.Family().IsIP() {
		return []net.IP{c.destination.Address.IP()}
	}
	return nil
}
