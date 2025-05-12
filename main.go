package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"naevis/globals"
	"naevis/ratelim"
	"naevis/routes"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
	quic "github.com/quic-go/quic-go"
	"github.com/rs/cors"
)

// Middleware: Security headers
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "max-age=0, no-cache, no-store, must-revalidate, private")
		next.ServeHTTP(w, r)
	})
}

// Middleware: Simple request logging
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] %s %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// Health check endpoint
func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "200")
}

func acceptClients(ctx context.Context) {
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		log.Fatalf("[GATEWAY] tls cert load: %v", err)
	}
	listener, err := quic.ListenAddr(globals.QuicListenAddr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{globals.QuicProto},
	}, nil)
	if err != nil {
		log.Fatalf("[GATEWAY] quic listen: %v", err)
	}
	log.Printf("[GATEWAY] QUIC listening on %s", globals.QuicListenAddr)

	go func() {
		<-ctx.Done()
		log.Println("[GATEWAY] Shutting down QUIC listener...")
		listener.Close()
	}()

	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("[GATEWAY] accept error: %v", err)
			}
			continue
		}
		globals.ConnMu.Lock()
		if globals.ActiveConn != nil {
			globals.ActiveConn.CloseWithError(0, "new client connected")
		}
		globals.ActiveConn = sess
		globals.ConnMu.Unlock()
		log.Printf("[GATEWAY] client connected: %s", sess.RemoteAddr())
	}
}

// Set up all routes and middleware layers
func setupRouter(rateLimiter *ratelim.RateLimiter) http.Handler {
	router := httprouter.New()
	router.GET("/health", Index)

	routes.AddProxyRoutes(router, rateLimiter)

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Restrict in production
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	return loggingMiddleware(securityHeaders(c.Handler(router)))
}

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found. Continuing with system environment variables.")
	}

	// Setup graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start QUIC listener in a goroutine
	go acceptClients(ctx)

	// Set up rate limiter and HTTP server
	rateLimiter := ratelim.NewRateLimiter()
	handler := setupRouter(rateLimiter)

	server := &http.Server{
		Addr:              globals.HttpListenAddr,
		Handler:           handler,
		ReadTimeout:       7 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	// Register shutdown logic
	server.RegisterOnShutdown(func() {
		log.Println("🛑 Cleaning up resources before shutdown...")
		// Add any DB or connection cleanup here
	})

	// Start HTTP server
	go func() {
		log.Printf("🚀 HTTP server listening on %s", globals.HttpListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	stop() // release signal notify resources

	log.Println("🛑 Shutdown signal received. Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("❌ Server shutdown failed: %v", err)
	}

	log.Println("✅ Server stopped cleanly")
}
