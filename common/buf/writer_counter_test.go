package buf_test

import (
	"bytes"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"

	appstats "github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/transport/internet/stat"
)

// 模拟带写入返回值的 TCP 连接，经过 NewWriter 的真实统计包装拆解路径。
type countedWriterConn struct {
	net.Conn
	write func([]byte) (int, error)
}

func (c *countedWriterConn) Write(p []byte) (int, error) {
	return c.write(p)
}

func (c *countedWriterConn) SyscallConn() (syscall.RawConn, error) {
	return nil, errors.New("测试连接没有底层套接字")
}

func newCountedWriter(t *testing.T, write func([]byte) (int, error)) (*buf.BufferToBytesWriter, *appstats.Counter) {
	t.Helper()
	counter := &appstats.Counter{}
	writer := buf.NewWriter(&stat.CounterConnection{
		Connection:   &countedWriterConn{write: write},
		WriteCounter: counter,
	})
	bytesWriter, ok := writer.(*buf.BufferToBytesWriter)
	if !ok {
		t.Fatalf("未进入 TCP 字节写入路径: %T", writer)
	}
	return bytesWriter, counter
}

func TestBufferedWriterCountsBytes(t *testing.T) {
	var destination bytes.Buffer
	writer, counter := newCountedWriter(t, destination.Write)
	buffered := buf.NewBufferedWriter(writer)
	assertCount := func() {
		t.Helper()
		if got, want := counter.Value(), int64(destination.Len()); got != want {
			t.Fatalf("计数 = %d，实际已写入 = %d", got, want)
		}
	}
	if _, err := buffered.Write([]byte("request-header")); err != nil {
		t.Fatal(err)
	}
	if counter.Value() != 0 {
		t.Fatal("仍在缓冲区中的字节被提前计数")
	}
	if err := buffered.SetBuffered(false); err != nil {
		t.Fatal(err)
	}
	assertCount()
	if _, err := buffered.Write([]byte("request-body")); err != nil {
		t.Fatal(err)
	}
	assertCount()

	// 单缓冲区与多个缓冲区原有的计数路径不能因新增 Write 再累计一次。
	for _, parts := range [][]string{{"single"}, {"first", "second"}} {
		var multi buf.MultiBuffer
		for _, part := range parts {
			b := buf.New()
			if _, err := b.Write([]byte(part)); err != nil {
				b.Release()
				buf.ReleaseMulti(multi)
				t.Fatal(err)
			}
			multi = append(multi, b)
		}
		if err := buffered.WriteMultiBuffer(multi); err != nil {
			t.Fatal(err)
		}
		assertCount()
	}
	for range 2 {
		if err := buffered.Flush(); err != nil {
			t.Fatal(err)
		}
		assertCount()
	}
}

func TestBufferedWriterCountsAutomaticFlush(t *testing.T) {
	var destination bytes.Buffer
	writer, counter := newCountedWriter(t, destination.Write)
	buffered := buf.NewBufferedWriter(writer)
	payload := bytes.Repeat([]byte("x"), buf.Size+7)
	if n, err := buffered.Write(payload[:buf.Size]); err != nil || n != buf.Size {
		t.Fatalf("写入返回 = %d, %v", n, err)
	}
	if counter.Value() != int64(buf.Size) {
		t.Fatalf("自动刷新计数 = %d，期望 %d", counter.Value(), buf.Size)
	}
	if n, err := buffered.Write(payload[buf.Size:]); err != nil || n != 7 {
		t.Fatalf("尾部写入返回 = %d, %v", n, err)
	}
	if counter.Value() != int64(buf.Size) {
		t.Fatal("尾部尚未刷新时被提前计数")
	}
	if err := buffered.Flush(); err != nil {
		t.Fatal(err)
	}
	if counter.Value() != int64(len(payload)) || !bytes.Equal(destination.Bytes(), payload) {
		t.Fatal("尾部刷新后计数或内容不匹配")
	}
}

func TestBufferToBytesWriterCountsActualWrites(t *testing.T) {
	writeError := errors.New("测试写入失败")
	for _, tc := range []struct {
		name string
		n    int
		err  error
	}{
		{"完整写入", 6, nil},
		{"部分写入", 2, nil},
		{"部分写入后失败", 2, writeError},
		{"未写入即失败", 0, writeError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer, counter := newCountedWriter(t, func([]byte) (int, error) {
				return tc.n, tc.err
			})
			n, err := writer.Write([]byte("upload"))
			if n != tc.n || err != tc.err || counter.Value() != int64(tc.n) {
				t.Fatalf("返回 = %d, %v，计数 = %d；期望 = %d, %v", n, err, counter.Value(), tc.n, tc.err)
			}
		})
	}
}

func TestBufferedWriterCountsFailedFlush(t *testing.T) {
	writer, counter := newCountedWriter(t, func([]byte) (int, error) {
		return 3, io.ErrClosedPipe
	})
	buffered := buf.NewBufferedWriter(writer)
	if _, err := buffered.Write([]byte("upload")); err != nil {
		t.Fatal(err)
	}
	if err := buffered.Flush(); err != io.ErrClosedPipe {
		t.Fatalf("刷新错误 = %v", err)
	}
	if counter.Value() != 3 {
		t.Fatalf("刷新失败后计数 = %d，期望实际写入的 3 字节", counter.Value())
	}
	if err := buffered.Flush(); err != nil || counter.Value() != 3 {
		t.Fatal("重复刷新改变了失败写入的计数")
	}
}

func TestBufferToBytesWriterWithoutCounter(t *testing.T) {
	var destination bytes.Buffer
	writer := buf.NewWriter(&countedWriterConn{write: destination.Write}).(io.Writer)
	payload := []byte("uncounted")
	if n, err := writer.Write(payload); err != nil || n != len(payload) || !bytes.Equal(destination.Bytes(), payload) {
		t.Fatalf("无计数器写入失败: n=%d, err=%v", n, err)
	}
}
