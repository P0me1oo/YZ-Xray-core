package hysteria

import (
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

func testUDPSession(t *testing.T) (*udpSessionManager, *InterConn) {
	t.Helper()
	m := &udpSessionManager{m: make(map[uint32]*InterConn)}
	c := &InterConn{id: 7, ch: make(chan []byte, 1), time: time.Now()}
	c.close = func() {
		m.Lock()
		m.close(c)
		m.Unlock()
	}
	m.m[c.id] = c
	t.Cleanup(func() { _ = c.Close() })
	return m, c
}

func TestUDPSessionManagerCleanStopsAfterShutdown(t *testing.T) {
	m := &udpSessionManager{m: make(map[uint32]*InterConn)}
	started, done := make(chan struct{}), make(chan struct{})
	m.Lock()
	go func() {
		close(started)
		m.clean()
		close(done)
	}()
	<-started
	// 模拟 run 在数据报接收结束后持有管理器锁发布关闭状态。
	m.closed = true
	m.Unlock()
	select {
	case <-done:
	case <-time.After(3 * idleCleanupInterval):
		t.Fatal("管理器关闭后清理任务没有退出")
	}
	if _, err := m.udp(); err == nil {
		t.Fatal("关闭后仍允许创建 UDP 会话")
	}
}

func TestInterConnWriteConcurrentClose(t *testing.T) {
	_, c := testUDPSession(t)
	started, done := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	c.write = func([]byte) error {
		once.Do(func() { close(started) })
		runtime.Gosched()
		return nil
	}
	go func() {
		packet := make([]byte, 8)
		for {
			if _, err := c.Write(packet); err != nil {
				done <- err
				return
			}
		}
	}()
	<-started
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("关闭后的写入错误 = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("关闭后写入仍未停止")
	}
}

func TestUDPSessionManagerCleansExpiredSession(t *testing.T) {
	m, c := testUDPSession(t)
	m.udpIdleTimeout = time.Second
	c.time = time.Now().Add(-time.Hour)
	done := make(chan struct{})
	go func() {
		m.clean()
		close(done)
	}()
	t.Cleanup(func() {
		m.Lock()
		m.closed = true
		m.Unlock()
		select {
		case <-done:
		case <-time.After(3 * idleCleanupInterval):
			t.Error("清理任务没有退出")
		}
	})
	select {
	case _, ok := <-c.ch:
		if ok {
			t.Fatal("过期会话的接收通道仍然开启")
		}
	case <-time.After(3 * idleCleanupInterval):
		t.Fatal("过期会话没有关闭")
	}
	m.RLock()
	remaining := len(m.m)
	m.RUnlock()
	if remaining != 0 {
		t.Fatal("过期会话仍留在管理器中")
	}
}

func TestInterConnCloseDoesNotWaitForWrite(t *testing.T) {
	_, c := testUDPSession(t)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	finishWrite := func() { once.Do(func() { close(release) }) }
	t.Cleanup(finishWrite)
	wantErr := errors.New("测试数据报写入失败")
	c.write = func([]byte) error {
		close(started)
		<-release
		return wantErr
	}
	writeDone, readDone := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := c.Write(make([]byte, 8))
		writeDone <- err
	}()
	go func() {
		_, err := c.Read(make([]byte, 8))
		readDone <- err
	}()
	<-started
	closeDone := make(chan struct{})
	go func() {
		_ = c.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("关闭被正在进行的写入阻塞")
	}
	select {
	case err := <-readDone:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("关闭后读取错误 = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("关闭没有唤醒读取")
	}
	finishWrite()
	if err := <-writeDone; !errors.Is(err, wantErr) {
		t.Fatalf("底层写入错误未保留: %v", err)
	}
}

func TestInterConnBufferedReadAndRepeatedClose(t *testing.T) {
	m, c := testUDPSession(t)
	writes := 0
	c.write = func(packet []byte) error {
		writes++
		if binary.BigEndian.Uint32(packet) != c.id {
			t.Error("UDP 会话编号未写入数据报")
		}
		return nil
	}
	if n, err := c.Write(make([]byte, 8)); n != 8 || err != nil {
		t.Fatalf("正常写入 = %d, %v", n, err)
	}
	c.ch <- []byte("packet")
	for range 3 {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if len(m.m) != 0 {
		t.Fatal("关闭后的会话仍留在管理器中")
	}
	packet := make([]byte, 16)
	if n, err := c.Read(packet); err != nil || string(packet[:n]) != "packet" {
		t.Fatalf("关闭时已缓存的数据报丢失: n=%d, err=%v", n, err)
	}
	if _, err := c.Read(packet); !errors.Is(err, io.EOF) {
		t.Fatalf("缓存耗尽后的读取错误 = %v", err)
	}
	if _, err := c.Write(packet); !errors.Is(err, io.ErrClosedPipe) || writes != 1 {
		t.Fatalf("关闭后仍调用底层写入: writes=%d, err=%v", writes, err)
	}
}
