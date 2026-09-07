package singbridge

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	B "github.com/sagernet/sing/common/buf"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/signal"
)

type packetReadFunc func() (buf.MultiBuffer, error)

func (f packetReadFunc) ReadMultiBuffer() (buf.MultiBuffer, error) {
	return f()
}

func testPacketWrapper(t *testing.T, reader buf.Reader) *PacketConnWrapper {
	t.Helper()
	timer := signal.CancelAfterInactivity(context.Background(), func() {}, time.Hour)
	t.Cleanup(func() { timer.SetTimeout(0) })
	wrapper := &PacketConnWrapper{
		Reader: reader,
		Dest:   net.UDPDestination(net.LocalHostIP, 443),
		T:      timer,
	}
	t.Cleanup(func() { _ = wrapper.Close() })
	return wrapper
}

func testPacket(payload string) *buf.Buffer {
	packet := buf.New()
	_, _ = packet.Write([]byte(payload))
	return packet
}

func TestPacketConnWrapperBufferedPacketsAndError(t *testing.T) {
	first, second := testPacket("first"), testPacket("second")
	destination := net.UDPDestination(net.LocalHostIP, 8443)
	second.UDP = &destination
	reads := 0
	wrapper := testPacketWrapper(t, packetReadFunc(func() (buf.MultiBuffer, error) {
		reads++
		if reads > 1 {
			t.Error("已返回读取错误后不应再次读取底层连接")
			return nil, io.EOF
		}
		return buf.MultiBuffer{first, second}, io.EOF
	}))
	buffer := B.NewSize(buf.Size)
	defer buffer.Release()
	for i, want := range []string{"first", "second"} {
		buffer.Reset()
		addr, err := wrapper.ReadPacket(buffer)
		if err != nil || string(buffer.Bytes()) != want {
			t.Fatalf("packet %d: payload=%q, err=%v", i, buffer.Bytes(), err)
		}
		wantPort := uint16(443)
		if i == 1 {
			wantPort = 8443
		}
		if addr.Port != wantPort {
			t.Fatalf("packet %d: port=%d, want=%d", i, addr.Port, wantPort)
		}
	}
	buffer.Reset()
	if _, err := wrapper.ReadPacket(buffer); !errors.Is(err, io.EOF) {
		t.Fatalf("缓存耗尽后应返回原读取错误: %v", err)
	}
}

func TestPacketConnWrapperCloseDuringRead(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finishRead := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(finishRead)
	first, second := testPacket("first"), testPacket("second")
	wrapper := testPacketWrapper(t, packetReadFunc(func() (buf.MultiBuffer, error) {
		close(started)
		<-release
		return buf.MultiBuffer{first, second}, nil
	}))
	done := make(chan error, 1)
	go func() {
		buffer := B.NewSize(buf.Size)
		defer buffer.Release()
		_, err := wrapper.ReadPacket(buffer)
		done <- err
	}()
	<-started
	closed := make(chan struct{})
	go func() {
		_ = wrapper.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		finishRead()
		t.Fatal("关闭不能等待阻塞中的读取")
	}
	finishRead()
	if err := <-done; !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("关闭后到达的数据应被释放: %v", err)
	}
	if len(wrapper.cached) != 0 || !first.IsEmpty() || !second.IsEmpty() {
		t.Fatal("关闭后仍有未释放的 UDP 缓冲")
	}
}

func TestPacketConnWrapperConcurrentReadAndClose(t *testing.T) {
	for range 64 {
		wrapper := testPacketWrapper(t, packetReadFunc(func() (buf.MultiBuffer, error) {
			return nil, io.EOF
		}))
		first, second := testPacket("first"), testPacket("second")
		wrapper.cached = buf.MultiBuffer{first, second}
		start := make(chan struct{})
		var workers sync.WaitGroup
		for range 2 {
			workers.Go(func() {
				buffer := B.NewSize(buf.Size)
				defer buffer.Release()
				<-start
				_, err := wrapper.ReadPacket(buffer)
				if err != nil && !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("read: %v", err)
				}
			})
			workers.Go(func() {
				<-start
				if err := wrapper.Close(); err != nil {
					t.Errorf("close: %v", err)
				}
			})
		}
		close(start)
		workers.Wait()
		if len(wrapper.cached) != 0 || !first.IsEmpty() || !second.IsEmpty() {
			t.Fatal("重复关闭后仍有未释放的 UDP 缓冲")
		}
		buffer := B.NewSize(buf.Size)
		_, err := wrapper.ReadPacket(buffer)
		buffer.Release()
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("关闭后的读取应失败: %v", err)
		}
	}
}
