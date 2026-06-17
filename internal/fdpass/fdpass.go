// Package fdpass provides file descriptor passing over Unix domain
// sockets using SCM_RIGHTS. It is designed for SOCK_SEQPACKET sockets
// which provide reliable message boundaries.
package fdpass

import (
	"errors"
	"net"
	"syscall"
)

// SendFd sends a file descriptor over a Unix domain socket connection
// using SCM_RIGHTS. The connection should use SOCK_SEQPACKET for
// reliable message boundaries.
func SendFd(conn *net.UnixConn, fd int) error {
	rights := syscall.UnixRights(fd)
	_, _, err := conn.WriteMsgUnix([]byte{0}, rights, nil)
	return err
}

// ReceiveFd receives a file descriptor over a Unix domain socket
// connection using SCM_RIGHTS. The connection should use
// SOCK_SEQPACKET for reliable message boundaries.
func ReceiveFd(conn *net.UnixConn) (int, error) {
	buf := make([]byte, 32)
	oob := make([]byte, 32)
	_, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return -1, err
	}
	scms, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return -1, err
	}
	if len(scms) == 0 {
		return -1, errors.New("no socket control message received")
	}
	rights, err := syscall.ParseUnixRights(&scms[0])
	if err != nil {
		return -1, err
	}
	if len(rights) == 0 {
		return -1, errors.New("no file descriptor received")
	}
	return rights[0], nil
}
