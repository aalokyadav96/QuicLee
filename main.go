package main

import (
	"encoding/json"
	"log"
	"net/http"

	_ "github.com/glebarez/go-sqlite"
	"github.com/quic-go/quic-go/http3"
)

func main() {
	// Create our HTTP mux with the event handler.
	mux := http.NewServeMux()
	mux.HandleFunc("/", Index)

	// Start the normal HTTP server in a separate goroutine.
	go func() {
		log.Println("HTTP server listening on port 4000...")
		log.Fatal(http.ListenAndServe(":4000", mux))
	}()

	// Start the QUIC server using TLS.
	quicServer := &http3.Server{
		Addr:    ":4433",
		Handler: mux,
	}

	log.Println("QUIC server listening on port 4433...")
	log.Fatal(quicServer.ListenAndServeTLS("cert.pem", "key.pem"))
}

func Index(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode("Hi")
}
