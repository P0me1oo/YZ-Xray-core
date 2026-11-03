//go:build linux

package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtls/xray-core/app/dispatcher"
	appstats "github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/signal"
	"golang.org/x/time/rate"
)

func rawCopyTCPPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialTCP("tcp", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	server, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	for _, conn := range []*net.TCPConn{client, server} {
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	return client, server
}

type rawCopyObservedWriter struct {
	buf.Writer
	writes atomic.Int64
}

func (w *rawCopyObservedWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.writes.Add(1)
	return w.Writer.WriteMultiBuffer(mb)
}
func (w *rawCopyObservedWriter) UnwrapWriter() buf.Writer { return w.Writer }

func TestRawCopySpliceUpdatesNestedCountersWhileOpen(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "unlimited", true: "limited"}[limited], func(t *testing.T) {
			const size = 64*1024 + 3
			producer, source := rawCopyTCPPair(t)
			destination, consumer := rawCopyTCPPair(t)
			user, relay, readCounter, writeCounter := new(appstats.Counter), new(appstats.Counter), new(appstats.Counter), new(appstats.Counter)
			observed := &rawCopyObservedWriter{Writer: buf.NewWriter(destination)}
			var writer buf.Writer = &dispatcher.SizeStatWriter{Counter: relay, Writer: observed}
			writer = &dispatcher.SizeStatWriter{Counter: user, Writer: writer}
			limiter := rate.NewLimiter(0, 4*size)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if limited {
				writer = &dispatcher.RateLimitWriter{Writer: writer, Limiter: limiter, Context: ctx}
			}
			ctx = session.ContextWithInbound(ctx, &session.Inbound{CanSpliceCopy: 1})
			ctx = session.ContextWithOutbounds(ctx, []*session.Outbound{{CanSpliceCopy: 1}})
			timer := signal.CancelAfterInactivity(ctx, cancel, time.Minute)
			defer timer.SetTimeout(0)
			done := make(chan error, 1)
			go func() {
				// Include transport counters exactly as CopyRawConnIfExist does after unwrapping.
				state := newRawCopyState(writer, readCounter, writeCounter, timer)
				handled, err := copyRawWithHandlers(ctx, source, destination, state)
				if !handled {
					err = errors.New("TCP 未进入内核 splice")
				}
				done <- err
			}()
			for round := 1; round <= 2; round++ {
				payload := bytes.Repeat([]byte{byte(round)}, size)
				if _, err := producer.Write(payload); err != nil {
					t.Fatal(err)
				}
				got := make([]byte, size)
				if _, err := io.ReadFull(consumer, got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, payload) {
					t.Fatal("splice 数据不完整")
				}
				want := int64(round * size)
				deadline := time.Now().Add(time.Second)
				for user.Value() != want || relay.Value() != want || writeCounter.Value() != want {
					if time.Now().After(deadline) {
						t.Fatalf("连接尚未关闭时计数未更新: user=%d relay=%d write=%d", user.Value(), relay.Value(), writeCounter.Value())
					}
					time.Sleep(time.Millisecond)
				}
				if readCounter.Value() != want {
					t.Fatalf("读取计数=%d，期望 %d", readCounter.Value(), want)
				}
				select {
				case err := <-done:
					t.Fatalf("未到 EOF 就结束: %v", err)
				default:
				}
				if limited && int64(limiter.Tokens()) != 4*size-want {
					t.Fatalf("限速未按实际字节扣减: %.0f", limiter.Tokens())
				}
			}
			if err := producer.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if observed.writes.Load() != 0 {
				t.Fatal("快速复制回退到程序缓冲区")
			}
			// 单方向 EOF 后仍可反向传输，不因清理取消回调关闭整条 TCP。
			if _, err := consumer.Write([]byte("up")); err != nil {
				t.Fatal(err)
			}
			back := make([]byte, 2)
			if _, err := io.ReadFull(destination, back); err != nil || string(back) != "up" {
				t.Fatalf("半关闭破坏反向传输: %q %v", back, err)
			}
		})
	}
}

func TestRawCopySpliceWaitsForBandwidthBeforeForwarding(t *testing.T) {
	producer, source := rawCopyTCPPair(t)
	destination, consumer := rawCopyTCPPair(t)
	const size = 4096
	limiter := rate.NewLimiter(10*1024, 1024)
	if !limiter.AllowN(time.Now(), 1024) {
		t.Fatal("无法耗尽初始额度")
	}
	writer := &dispatcher.RateLimitWriter{Writer: buf.NewWriter(destination), Limiter: limiter, Context: t.Context()}
	done := make(chan error, 1)
	go func() {
		handled, err := copyRawWithHandlers(t.Context(), source, destination, newRawCopyState(writer, nil, nil, nil))
		if !handled {
			err = errors.New("有限速连接没有使用 splice")
		}
		done <- err
	}()
	started := time.Now()
	if _, err := producer.Write(bytes.Repeat([]byte("x"), size)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(consumer, make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 300*time.Millisecond {
		t.Fatalf("快速复制绕过了转发前限速: %s", elapsed)
	}
	producer.CloseWrite()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRawCopySpliceCancellationStopsWaitingReaderAndLimiter(t *testing.T) {
	for _, waitingLimiter := range []bool{false, true} {
		t.Run(map[bool]string{false: "reader", true: "limiter"}[waitingLimiter], func(t *testing.T) {
			producer, source := rawCopyTCPPair(t)
			destination, consumer := rawCopyTCPPair(t)
			counter := new(appstats.Counter)
			var writer buf.Writer = &dispatcher.SizeStatWriter{Counter: counter, Writer: buf.NewWriter(destination)}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if waitingLimiter {
				limiter := rate.NewLimiter(0, 1024)
				if !limiter.AllowN(time.Now(), 1024) {
					t.Fatal("限速准备失败")
				}
				writer = &dispatcher.RateLimitWriter{Writer: writer, Limiter: limiter, Context: ctx}
				if _, err := producer.Write(bytes.Repeat([]byte("x"), 2048)); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				handled, err := copyRawWithHandlers(ctx, source, destination, newRawCopyState(writer, nil, nil, nil))
				if !handled {
					err = errors.New("TCP 未进入 splice")
				}
				done <- err
			}()
			time.AfterFunc(20*time.Millisecond, cancel)
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("取消错误=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("取消未中断内核等待")
			}
			if counter.Value() != 0 {
				t.Fatalf("未转发数据却增加计数=%d", counter.Value())
			}
			_ = consumer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _ := consumer.Read(make([]byte, 1))
			if n != 0 {
				t.Fatal("限速取消后仍写出字节")
			}
		})
	}
}

type rawCopyFailingWriter struct {
	buf.Writer
	calls   int
	written int64
	failure error
}

func (w *rawCopyFailingWriter) UnwrapWriter() buf.Writer { return w.Writer }
func (w *rawCopyFailingWriter) BeforeRawCopy(context.Context, int) error {
	w.calls++
	if w.calls == 2 {
		return w.failure
	}
	return nil
}
func (w *rawCopyFailingWriter) AfterRawCopy(n int64) { w.written += n }

func TestRawCopySpliceErrorKeepsOnlyForwardedPrefix(t *testing.T) {
	producer, source := rawCopyTCPPair(t)
	destination, consumer := rawCopyTCPPair(t)
	defer consumer.Close()
	failure := errors.New("阻止下一段复制")
	first, second := new(appstats.Counter), new(appstats.Counter)
	writer := &rawCopyFailingWriter{Writer: &dispatcher.SizeStatWriter{
		Counter: first, Writer: &dispatcher.SizeStatWriter{Counter: second, Writer: buf.NewWriter(destination)},
	}, failure: failure}
	if _, err := producer.Write(bytes.Repeat([]byte("x"), 128*1024)); err != nil {
		t.Fatal(err)
	}
	handled, err := copyRawWithHandlers(t.Context(), source, destination, newRawCopyState(writer, nil, nil, nil))
	if !handled || !errors.Is(err, failure) {
		t.Fatalf("错误未传递: handled=%t err=%v", handled, err)
	}
	if writer.written <= 0 || first.Value() != writer.written || second.Value() != writer.written {
		t.Fatalf("前缀计数错误: actual=%d first=%d second=%d", writer.written, first.Value(), second.Value())
	}
	received := make([]byte, writer.written)
	if _, err := io.ReadFull(consumer, received); err != nil {
		t.Fatal(err)
	}
}

func TestRawCopyUnsupportedSourceDoesNotConsumeData(t *testing.T) {
	producer, source := net.Pipe()
	defer producer.Close()
	defer source.Close()
	destination, _ := rawCopyTCPPair(t)
	handled, err := copyRawWithHandlers(t.Context(), source, destination, newRawCopyState(buf.Discard, nil, nil, nil))
	if handled || err != nil {
		t.Fatalf("不支持的来源无法回退: %t %v", handled, err)
	}
	done := make(chan error, 1)
	go func() { _, err := producer.Write([]byte("fallback")); done <- err }()
	received := make([]byte, 8)
	if _, err := io.ReadFull(source, received); err != nil {
		t.Fatal(err)
	}
	if string(received) != "fallback" {
		t.Fatal("回退前消耗了数据")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
