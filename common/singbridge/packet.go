package singbridge

import (
	"context"
	"io"
	"sync"
	"time"

	B "github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/signal"
	"github.com/xtls/xray-core/transport"
)

func CopyPacketConn(ctx context.Context, inboundConn net.Conn, link *transport.Link, destination net.Destination, serverConn net.PacketConn) error {
	cancel := func() {
		common.Interrupt(link.Reader)
		common.Interrupt(serverConn)
	}
	conn := &PacketConnWrapper{
		Reader: link.Reader,
		Writer: link.Writer,
		Dest:   destination,
		Conn:   inboundConn,
		T:      signal.CancelAfterInactivity(ctx, cancel, 300*time.Second),
	}
	return ReturnError(bufio.CopyPacketConn(ctx, conn, bufio.NewPacketConn(serverConn)))
}

type PacketConnWrapper struct {
	buf.Reader
	buf.Writer
	net.Conn
	Dest        net.Destination
	readAccess  sync.Mutex
	cacheAccess sync.Mutex
	cached      buf.MultiBuffer
	readErr     error
	closed      bool

	// A simple patch to avoid goroutine leak since sing infra cannot awake read block by write err
	T *signal.ActivityTimer
}

func (w *PacketConnWrapper) ReadPacket(buffer *B.Buffer) (addr M.Socksaddr, err error) {
	w.readAccess.Lock()
	defer w.readAccess.Unlock()
	w.T.Update()
	defer func() {
		if err != nil {
			// uplinkonly
			w.T.SetTimeout(2 * time.Second)
		}
	}()
	w.cacheAccess.Lock()
	if w.closed {
		w.cacheAccess.Unlock()
		return M.Socksaddr{}, io.ErrClosedPipe
	}
	var bb *buf.Buffer
	w.cached, bb = buf.SplitFirst(w.cached)
	w.cacheAccess.Unlock()
	if bb == nil {
		if w.readErr != nil {
			return M.Socksaddr{}, w.readErr
		}
		// 阻塞读取不持有缓存锁，关闭可以立即回收已有数据。
		mb, readErr := w.ReadMultiBuffer()
		w.cacheAccess.Lock()
		if w.closed {
			w.cacheAccess.Unlock()
			buf.ReleaseMulti(mb)
			return M.Socksaddr{}, io.ErrClosedPipe
		}
		w.cached, bb = buf.SplitFirst(mb)
		w.readErr = readErr
		w.cacheAccess.Unlock()
		if bb == nil {
			return M.Socksaddr{}, readErr
		}
	}
	defer bb.Release()
	if _, err = buffer.Write(bb.Bytes()); err != nil {
		return M.Socksaddr{}, err
	}
	destination := w.Dest
	if bb.UDP != nil {
		destination = *bb.UDP
	}
	return ToSocksaddr(destination), nil
}

func (w *PacketConnWrapper) WritePacket(buffer *B.Buffer, destination M.Socksaddr) (err error) {
	w.T.Update()
	defer func() {
		if err != nil {
			// downlinkonly
			w.T.SetTimeout(5 * time.Second)
		}
	}()
	endpoint, err := ToDestination(destination, net.Network_UDP)
	if err != nil {
		return err
	}
	vBuf := buf.New()
	vBuf.Write(buffer.Bytes())
	vBuf.UDP = &endpoint
	return w.WriteMultiBuffer(buf.MultiBuffer{vBuf})
}

func (w *PacketConnWrapper) Close() error {
	w.cacheAccess.Lock()
	defer w.cacheAccess.Unlock()
	w.closed = true
	buf.ReleaseMulti(w.cached)
	w.cached = nil
	return nil
}
