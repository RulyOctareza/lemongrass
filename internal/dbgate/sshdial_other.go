//go:build !unix

package dbgate

import (
	"errors"
	"net"
	"os"
)

func socketPair() (net.Conn, *os.File, error) {
	return nil, nil, errors.New("dbgate: ssh connections are not supported on this platform")
}
