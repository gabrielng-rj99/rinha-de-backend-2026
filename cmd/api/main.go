package main

import (
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"rinha-backend-2026/internal/fdpass"
	rinhttp "rinha-backend-2026/internal/http"
	"rinha-backend-2026/internal/index"
	"rinha-backend-2026/internal/vector"
)

const (
	// PR_SET_TIMERSLACK is the Linux prctl option (not defined in Go's syscall).
	PR_SET_TIMERSLACK = 29

	// Default paths.
	defaultIndexPath  = "/data/references.idx"
	defaultSocketPath = "/sockets/api.sock"

	// Read buffer size (shared via sync.Pool).
	bufferSize = 8192
)

func main() {
	// ---- Configuration ----
	indexPath := envOr("INDEX_PATH", defaultIndexPath)
	instance := os.Getenv("INSTANCE")

	socketPath := os.Getenv("SOCKET_PATH")
	if socketPath == "" {
		switch instance {
		case "1":
			socketPath = "/sockets/api1.sock"
		case "2":
			socketPath = "/sockets/api2.sock"
		default:
			socketPath = defaultSocketPath
		}
	}

	// ---- GC tuning ----
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(160 * 1024 * 1024) // 160 MB
	runtime.GC()

	// ---- Kernel scheduler optimisation ----
	if _, _, err := syscall.Syscall6(syscall.SYS_PRCTL, PR_SET_TIMERSLACK, 1, 0, 0, 0, 0); err != 0 && err != syscall.ENOSYS {
		log.Printf("warn: prctl(PR_SET_TIMERSLACK) failed: %v", err)
	}

	// ---- Load index ----
	log.Printf("loading index from %s ...", indexPath)
	idx, err := index.LoadIndex(indexPath)
	if err != nil {
		log.Fatalf("failed to load index: %v", err)
	}
	defer idx.Close()
	log.Printf("index loaded: %d centroids, %d vectors", len(idx.Centroids), len(idx.Vectors))

	// ---- Warmup ----
	log.Print("running warmup (2048 synthetic queries)...")
	warmup(idx)
	log.Print("warmup complete")

	// ---- Unix socket (SOCK_SEQPACKET) ----
	if err := syscall.Unlink(socketPath); err != nil && !os.IsNotExist(err) {
		log.Printf("warn: unlink %s: %v", socketPath, err)
	}

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		log.Fatalf("socket: %v", err)
	}

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		log.Printf("warn: setsockopt SO_REUSEADDR: %v", err)
	}

	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: socketPath}); err != nil {
		log.Fatalf("bind %s: %v", socketPath, err)
	}

	if err := syscall.Listen(fd, 128); err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer syscall.Close(fd)
	log.Printf("listening on %s (SOCK_SEQPACKET)", socketPath)

	// ---- Buffer pool (reduces allocations for request reads) ----
	var bufPool = sync.Pool{
		New: func() any {
			b := make([]byte, bufferSize)
			return b
		},
	}

	// ---- Signal handling (graceful shutdown) ----
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Print("received SIGTERM, shutting down...")
		syscall.Close(fd)
		// Give in-flight handlers a moment to finish.
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()

	// ---- Accept loop ----
	for {
		connFd, _, err := syscall.Accept(fd)
		if err != nil {
			if err == syscall.EBADF || err == syscall.EINVAL {
				// Listening socket closed — we're done.
				break
			}
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConnection(connFd, idx, &bufPool)
	}

	log.Print("server stopped")
}

// ---------------------------------------------------------------------------
// Connection handling
// ---------------------------------------------------------------------------

// handleConnection receives file descriptors from the load balancer via the
// SOCK_SEQPACKET Unix socket in a loop, then serves HTTP on each fd.
func handleConnection(connFd int, idx *index.Index, bufPool *sync.Pool) {
	defer syscall.Close(connFd)

	// Wrap the accepted SOCK_SEQPACKET fd as a *net.UnixConn so we can
	// call fdpass.ReceiveFd.
	f := os.NewFile(uintptr(connFd), "")
	conn, err := net.FileConn(f)
	f.Close() // net.FileConn dup's the fd internally; safe to close ours.
	if err != nil {
		log.Printf("fileconn(accept): %v", err)
		return
	}
	defer conn.Close()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		log.Printf("unexpected conn type %T from SOCK_SEQPACKET socket", conn)
		return
	}

	// Loop: receive multiple fds over the same Unix socket connection.
	for {
		clientFd, err := fdpass.ReceiveFd(unixConn)
		if err != nil {
			// Connection closed by LB or error — normal at shutdown.
			return
		}
		// Serve HTTP on the received fd in a goroutine so we can
		// immediately receive the next fd.
		go func(fd int) {
			serveHTTP(fd, idx, bufPool)
			syscall.Close(fd)
		}(clientFd)
	}
}

// serveHTTP reads HTTP/1.1 requests from the TCP fd, dispatches them,
// and writes responses. It loops for keep-alive.
func serveHTTP(fd int, idx *index.Index, bufPool *sync.Pool) {
	buf := bufPool.Get().([]byte)
	defer bufPool.Put(buf)

	for {
		n, err := syscall.Read(fd, buf)
		if err != nil {
			return
		}
		if n == 0 {
			return
		}

		data := buf[:n]
		for {
			method, path, body, bytesRead := rinhttp.ParseRequest(data)
			if method == "" {
				break
			}

			switch {
			case method == "GET" && path == "/ready":
				writeAll(fd, rinhttp.ReadyResponse)

			case method == "POST" && path == "/fraud-score":
				vec, err := vector.Normalize(body)
				if err != nil {
					writeAll(fd, badRequestResp())
					return
				}
				fc := idx.Search(&vec)
				writeAll(fd, rinhttp.FraudResponses[fc])

			default:
				return
			}

			// Advance past the consumed request (support pipelining).
			data = data[bytesRead:]
			if len(data) == 0 {
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Warmup
// ---------------------------------------------------------------------------

// warmup runs 2048 synthetic KNN searches to prime CPU caches, page tables,
// and branch predictors before the benchmark begins.
func warmup(idx *index.Index) {
	numVecs := len(idx.Vectors)
	if numVecs == 0 {
		return
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 2048; i++ {
		// Pick a base vector from the index (cyclically).
		src := &idx.Vectors[i%numVecs]

		var q [14]float32
		for d := 0; d < 14; d++ {
			v := src[d] + float32(rng.Intn(5)-2) // jitter ∈ [-2, +2]
			q[d] = v
		}

		idx.Search(&q)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeAll calls syscall.Write repeatedly until all of buf is written.
func writeAll(fd int, buf []byte) {
	for len(buf) > 0 {
		n, err := syscall.Write(fd, buf)
		if err != nil {
			return
		}
		buf = buf[n:]
	}
}

// badRequestResp returns a pre-built HTTP 400 response.
func badRequestResp() []byte {
	return []byte(
		"HTTP/1.1 400 Bad Request\r\n" +
			"Content-Length: 0\r\n" +
			"Connection: close\r\n" +
			"\r\n",
	)
}

// envOr returns the environment variable value, or defaultValue if unset.
func envOr(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
