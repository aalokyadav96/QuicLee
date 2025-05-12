package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"
)

const (
	// Where the worker will connect
	quicListenAddr = ":4433"
	// The ALPN proto for QUIC
	quicProto = "quic-api"
)

var (
	activeConn quic.Connection
	connMu     sync.RWMutex
)

// acceptClients runs in the background and populates `activeConn`.
func acceptClients() {
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		log.Fatal("tls cert load:", err)
	}
	listener, err := quic.ListenAddr(quicListenAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{quicProto},
	}, nil)
	if err != nil {
		log.Fatal("quic listen:", err)
	}
	log.Println("gateway: QUIC listening on", quicListenAddr)

	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			log.Println("gateway: accept error:", err)
			continue
		}
		connMu.Lock()
		if activeConn != nil {
			activeConn.CloseWithError(0, "new client connected")
		}
		activeConn = sess
		connMu.Unlock()
		log.Println("gateway: client connected:", sess.RemoteAddr())
	}
}

// writeFrame writes a 2-byte length prefix and then the payload.
func writeFrame(w io.Writer, data []byte) error {
	if len(data) > 0xFFFF {
		return io.ErrShortBuffer
	}
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// readFrame reads back a 2-byte length prefix and then the payload.
func readFrame(r io.Reader) ([]byte, error) {
	var lb [2]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(lb[:])
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	// Grab the active client connection
	connMu.RLock()
	sess := activeConn
	connMu.RUnlock()
	if sess == nil || sess.Context().Err() != nil {
		http.Error(w, "no backend client connected", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stream, err := sess.OpenStreamSync(ctx)
	if err != nil {
		http.Error(w, "failed to open QUIC stream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer stream.Close()

	// 1) method
	if err := writeFrame(stream, []byte(r.Method)); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// 2) path
	if err := writeFrame(stream, []byte(r.URL.Path)); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// 3) body (raw JSON, if any)
	if r.Body != nil {
		if _, err := io.Copy(stream, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	// half-close the write side to signal end-of-frame
	if err := stream.Close(); err != nil {
		log.Println("gateway warning: close write:", err)
	}

	// Read back a single JSON payload frame
	respBytes, err := io.ReadAll(stream)
	if err != nil {
		http.Error(w, "reading backend reply: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Proxy it back
	w.Header().Set("Content-Type", "application/json")
	w.Write(respBytes)
}

func main() {
	// Start QUIC acceptor
	go acceptClients()

	// Start HTTP API
	http.HandleFunc("/api", apiHandler)
	log.Println("gateway: HTTP listening on :4000")
	log.Fatal(http.ListenAndServe(":4000", nil))
}
