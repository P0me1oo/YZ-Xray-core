//go:build !linux

package proxy

import (
	"context"
	"net"
)

func copyRawWithHandlers(context.Context, net.Conn, *net.TCPConn, *rawCopyState) (bool, error) {
	return false, nil
}
