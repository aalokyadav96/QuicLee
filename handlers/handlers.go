package handlers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"naevis/globals"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
)

// fail logs and sends an HTTP error
func fail(w http.ResponseWriter, code int, msg string, err error) {
	log.Printf("[GATEWAY][ERROR] %s: %v", msg, err)
	http.Error(w, msg, code)
}

func ApiHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	globals.ConnMu.RLock()
	sess := globals.ActiveConn
	globals.ConnMu.RUnlock()
	if sess == nil || sess.Context().Err() != nil {
		fail(w, http.StatusServiceUnavailable, "no backend connected", nil)
		return
	}

	// propagate deadline from HTTP to QUIC
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stream, err := sess.OpenStreamSync(ctx)
	if err != nil {
		fail(w, http.StatusBadGateway, "open QUIC stream failed", err)
		return
	}
	defer stream.Close()

	// 1) method
	if err := WriteFrame(stream, []byte(r.Method)); err != nil {
		fail(w, http.StatusBadGateway, "write method frame", err)
		return
	}
	// 2) full path
	if err := WriteFrame(stream, []byte(r.URL.Path)); err != nil {
		fail(w, http.StatusBadGateway, "write path frame", err)
		return
	}
	// 3) headers
	hdrBytes, _ := json.Marshal(r.Header)
	if err := WriteFrame(stream, hdrBytes); err != nil {
		fail(w, http.StatusBadGateway, "write headers frame", err)
		return
	}
	// 4) body
	if r.Body != nil {
		if _, err := io.Copy(stream, r.Body); err != nil {
			fail(w, http.StatusBadGateway, "write body to QUIC", err)
			return
		}
	}
	// signal EOF
	stream.Close()

	// read entire JSON envelope reply
	reply, err := io.ReadAll(stream)
	if err != nil {
		fail(w, http.StatusBadGateway, "read backend reply", err)
		return
	}

	// proxy back
	for k, vs := range map[string][]string{"Content-Type": {"application/json"}} {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write(reply)
}

// readFrame reads a 2-byte length-prefixed frame from the reader.
func ReadFrame(r io.Reader) ([]byte, error) {
	var lb [2]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(lb[:])
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

// WriteFrame writes a 2-byte length-prefixed frame to the writer.
func WriteFrame(w io.Writer, data []byte) error {
	if len(data) > 65535 {
		return io.ErrShortWrite
	}
	var lb [2]byte
	binary.BigEndian.PutUint16(lb[:], uint16(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// writeJSON writes JSON-encoded data with length prefix.
func WriteJSON(w io.Writer, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return WriteFrame(w, b)
}
