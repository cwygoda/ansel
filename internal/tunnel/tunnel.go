package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sync"

	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

// FileServer serves local files via an ngrok tunnel.
type FileServer struct {
	listener ngrok.Tunnel
	server   *http.Server
	files    map[string]string // path -> local file path
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// New creates a new FileServer with an ngrok tunnel.
// If authToken is empty, it will use NGROK_AUTHTOKEN env var.
func New(ctx context.Context, authToken string) (*FileServer, error) {
	ctx, cancel := context.WithCancel(ctx)

	var opts []ngrok.ConnectOption
	if authToken != "" {
		opts = append(opts, ngrok.WithAuthtoken(authToken))
	}

	ln, err := ngrok.Listen(ctx, config.HTTPEndpoint(), opts...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start ngrok tunnel: %w", err)
	}

	fs := &FileServer{
		listener: ln,
		files:    make(map[string]string),
		ctx:      ctx,
		cancel:   cancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", fs.handleRequest)

	fs.server = &http.Server{Handler: mux}
	go func() {
		_ = fs.server.Serve(ln)
	}()

	return fs, nil
}

// URL returns the public URL of the tunnel.
func (s *FileServer) URL() string {
	return s.listener.URL()
}

// AddFile adds a local file to be served and returns its public URL.
func (s *FileServer) AddFile(localPath string) (string, error) {
	// Verify file exists
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	// Generate random path
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random path: %w", err)
	}
	randomPath := "/" + hex.EncodeToString(randomBytes) + ".jpg"

	s.mu.Lock()
	s.files[randomPath] = localPath
	s.mu.Unlock()

	return s.URL() + randomPath, nil
}

// handleRequest serves files from the map.
func (s *FileServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	localPath, ok := s.files[r.URL.Path]
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, localPath)
}

// Close shuts down the tunnel and server.
func (s *FileServer) Close() error {
	s.cancel()
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
