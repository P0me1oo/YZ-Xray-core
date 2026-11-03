//go:build linux

package proxy

import (
	"context"
	"io"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// Both halves use splice; bytes remain in the kernel pipe while limiters wait.
// A fresh pipe is drained before each read, so only the socket needs polling.
func copyRawWithHandlers(ctx context.Context, source net.Conn, destination *net.TCPConn, state *rawCopyState) (bool, error) {
	switch source.(type) {
	case *net.TCPConn, *net.UnixConn:
	default:
		return false, nil
	}
	socket, ok := source.(syscall.Conn)
	if !ok {
		return false, nil
	}
	reader, err := socket.SyscallConn()
	if err != nil {
		return true, err
	}
	writer, err := destination.SyscallConn()
	if err != nil {
		return true, err
	}
	var pipe [2]int
	if err := unix.Pipe2(pipe[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return false, nil
	}
	defer unix.Close(pipe[0])
	defer unix.Close(pipe[1])
	ctx, cleanup := state.context(ctx)
	defer cleanup()
	if err := ctx.Err(); err != nil {
		return true, err
	}
	stop := context.AfterFunc(ctx, func() {
		// Interrupt socket polling as well as limiter waits on cancellation.
		source.Close()
		destination.Close()
	})
	defer stop()

	started := false
	for {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		var count int64
		var syscallErr error
		size := state.chunkSize()
		err := reader.Read(func(fd uintptr) bool {
			for {
				n, spliceErr := unix.Splice(int(fd), nil, pipe[1], nil, size, unix.SPLICE_F_MOVE|unix.SPLICE_F_NONBLOCK)
				count, syscallErr = int64(n), spliceErr
				if syscallErr == unix.EINTR {
					continue
				}
				return syscallErr != unix.EAGAIN
			}
		})
		if canceled := ctx.Err(); canceled != nil {
			return true, canceled
		}
		if err != nil {
			return true, err
		}
		if syscallErr != nil {
			if !started && (syscallErr == unix.EINVAL || syscallErr == unix.ENOSYS || syscallErr == unix.EOPNOTSUPP) {
				return false, nil
			}
			return true, syscallErr
		}
		if count == 0 {
			return true, nil
		}
		started = true
		state.read(count)
		if err := state.before(ctx, int(count)); err != nil {
			return true, err
		}

		remaining := count
		err = writer.Write(func(fd uintptr) bool {
			for remaining > 0 {
				n, spliceErr := unix.Splice(pipe[0], nil, int(fd), nil, int(remaining), unix.SPLICE_F_MOVE|unix.SPLICE_F_NONBLOCK)
				written := int64(n)
				syscallErr = spliceErr
				if written > 0 {
					remaining -= written
					state.written(written)
				}
				if syscallErr == unix.EINTR {
					continue
				}
				if syscallErr == unix.EAGAIN {
					return false
				}
				if syscallErr != nil {
					return true
				}
				if written == 0 {
					syscallErr = io.ErrShortWrite
					return true
				}
			}
			return true
		})
		if canceled := ctx.Err(); canceled != nil {
			return true, canceled
		}
		if err != nil {
			return true, err
		}
		if syscallErr != nil {
			return true, syscallErr
		}
	}
}
