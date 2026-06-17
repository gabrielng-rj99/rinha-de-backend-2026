// Command lb is a load balancer that accepts TCP connections on
// port 9999 and forwards the underlying file descriptors to API
// instances via Unix domain sockets using SCM_RIGHTS. It does not
// read or inspect any payload data.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"rinha-backend-2026/internal/fdpass"
)

const (
	listenAddr = ":9999"
	api1Addr   = "/sockets/api1.sock"
	api2Addr   = "/sockets/api2.sock"
)

var (
	// apiConns holds the Unix connections to the two API instances.
	apiConns [2]*net.UnixConn
	// rrCounter is used for round-robin distribution.
	rrCounter atomic.Uint64
)

// dialUnixSeqpacket connects to a Unix domain socket using
// SOCK_SEQPACKET for reliable message boundaries. It returns a
// *net.UnixConn wrapping the raw socket.
func dialUnixSeqpacket(path string) (*net.UnixConn, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, err
	}
	addr := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Connect(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	conn, err := net.FileConn(file)
	file.Close()
	if err != nil {
		return nil, err
	}
	return conn.(*net.UnixConn), nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Connect to the two API instances (with retry).
	for i, addr := range []string{api1Addr, api2Addr} {
		name := fmt.Sprintf("api%d", i+1)
		for attempt := 0; ; attempt++ {
			var err error
			apiConns[i], err = dialUnixSeqpacket(addr)
			if err == nil {
				log.Printf("connected to %s at %s", name, addr)
				break
			}
			if attempt >= 60 {
				log.Fatalf("failed to connect to %s (%s) after 60 attempts: %v", name, addr, err)
			}
			if attempt == 0 || attempt%10 == 0 {
				log.Printf("waiting for %s at %s (attempt %d)...", name, addr, attempt+1)
			}
			time.Sleep(500 * time.Millisecond)
		}
		defer apiConns[i].Close()
	}

	// Create TCP listener with SO_REUSEADDR.
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
					log.Printf("setsockopt SO_REUSEADDR: %v", err)
				}
			})
		},
	}

	listener, err := lc.Listen(context.Background(), "tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", listenAddr, err)
	}
	defer listener.Close()

	log.Printf("load balancer listening on %s", listenAddr)
	log.Printf("forwarding connections to %s and %s", api1Addr, api2Addr)

	// --- Signal handling for graceful shutdown ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})

	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down...", sig)
		close(done)
		listener.Close()
	}()

	// --- Periodic connection count logging ---
	var connCount atomic.Uint64
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := connCount.Load()
				log.Printf("connections forwarded: %d", n)
			case <-done:
				return
			}
		}
	}()

	// --- Main accept loop ---
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-done:
				log.Println("accept loop exiting")
				return
			default:
				log.Printf("accept error: %v", err)
				continue
			}
		}
		go handleConn(conn, &connCount, done)
	}
}

// handleConn processes a single accepted TCP connection. It extracts
// the file descriptor from the connection and forwards it via
// SCM_RIGHTS to the next API instance in round-robin order.
func handleConn(conn net.Conn, connCount *atomic.Uint64, done chan struct{}) {
	defer conn.Close()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		log.Printf("expected *net.TCPConn, got %T", conn)
		return
	}

	// Enable TCP_NODELAY to minimize latency.
	if err := tcpConn.SetNoDelay(true); err != nil {
		log.Printf("SetNoDelay: %v", err)
		return
	}

	// Extract the underlying file descriptor.
	file, err := tcpConn.File()
	if err != nil {
		log.Printf("File: %v", err)
		return
	}
	fd := int(file.Fd())

	// Round-robin: select the next backend.
	idx := int(rrCounter.Add(1) - 1) % len(apiConns)

	// Forward the file descriptor to the chosen API instance.
	if err := fdpass.SendFd(apiConns[idx], fd); err != nil {
		log.Printf("SendFd to api%d: %v", idx+1, err)
		file.Close()
		return
	}

	// Close the LB's copy of the fd — the API instance now owns it.
	file.Close()

	connCount.Add(1)
}
