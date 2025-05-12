package globals

import (
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	RefreshTokenTTL = 7 * 24 * time.Hour // 7 days
	AccessTokenTTL  = 15 * time.Minute   // 15 minutes
)

var (
	// tokenSigningAlgo = jwt.SigningMethodHS256
	JwtSecret = []byte("your_secret_key") // Replace with a secure secret key
)

type ContextKey string

const UserIDKey ContextKey = "userId"

const (
	QuicListenAddr = ":4433"
	QuicProto      = "quic-api"
	HttpListenAddr = ":7000"
)

var (
	ActiveConn quic.Connection
	ConnMu     sync.RWMutex
)
