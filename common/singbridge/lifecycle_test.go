package singbridge

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/signal"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/pipe"
)

type lifecycleWriter struct {
	buf.Writer
	closed atomic.Int32
}

func (w *lifecycleWriter) Interrupt() {
	w.closed.Add(1)
	common.Interrupt(w.Writer)
}

func lifecycleLink(t *testing.T) (*transport.Link, *pipe.Writer, *lifecycleWriter) {
	t.Helper()
	r, w := pipe.New()
	ur, uw := pipe.New()
	t.Cleanup(func() { common.Interrupt(r); common.Interrupt(ur) })
	tracked := &lifecycleWriter{Writer: uw}
	return &transport.Link{Reader: r, Writer: tracked}, w, tracked
}

func waitLifecycle(t *testing.T, done <-chan struct{}, writer *lifecycleWriter) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("连接关闭后复制没有退出")
	}
	if n := writer.closed.Load(); n != 1 {
		t.Fatalf("关闭回调次数为 %d，期望 1", n)
	}
}

func TestCopyConnReleasesLink(t *testing.T) {
	for _, cancelContext := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect", true: "cancel"}[cancelContext], func(t *testing.T) {
			link, down, writer := lifecycleLink(t)
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { _ = CopyConn(ctx, server, link, server); close(done) }()
			if cancelContext {
				cancel()
			} else {
				_ = client.Close()
				_ = down.Close()
			}
			waitLifecycle(t, done, writer)
		})
	}
}

func TestCopyPacketConnReleasesLink(t *testing.T) {
	link, down, writer := lifecycleLink(t)
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	done := make(chan struct{})
	go func() {
		_ = CopyPacketConn(context.Background(), nil, link, xnet.UDPDestination(xnet.LocalHostIP, 443), server)
		close(done)
	}()
	_ = server.Close()
	_ = down.Close()
	waitLifecycle(t, done, writer)
}

func TestBridgeCleanupConcurrent(t *testing.T) {
	link, _, writer := lifecycleLink(t)
	cleanup := bridgeCleanup(link, nil)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() { defer wg.Done(); cleanup() }()
	}
	wg.Wait()
	if n := writer.closed.Load(); n != 1 {
		t.Fatalf("重复释放 %d 次", n)
	}
}

func TestBridgeTimeoutReleasesLink(t *testing.T) {
	link, _, writer := lifecycleLink(t)
	cleanup := bridgeCleanup(link, nil)
	timer := signal.CancelAfterInactivity(context.Background(), cleanup, time.Hour)
	// 触发与空闲超时相同的回调，再模拟复制结束的重复清理。
	timer.SetTimeout(0)
	cleanup()
	if n := writer.closed.Load(); n != 1 {
		t.Fatalf("超时关闭回调次数为 %d，期望 1", n)
	}
}
