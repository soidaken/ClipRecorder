// Package clipsync serves the PC clipboard to a paired phone on the local
// network. It uses plain HTTP with long polling: the phone asks for changes and
// the request stays open until something changes, which keeps the protocol and
// the dependencies tiny while still feeling instant.
package clipsync

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// maxText caps what a client may push, so a stray paste cannot fill the
// clipboard with megabytes.
const maxText = 1 << 20

// Store keeps the current clipboard text and its revision, and wakes the long
// polls waiting for a change.
type Store struct {
	mu      sync.Mutex
	rev     uint64
	text    string
	hash    string
	waiters map[chan struct{}]struct{}
}

func NewStore() *Store { return &Store{waiters: map[chan struct{}]struct{}{}} }

// Set replaces the text and returns the new revision. An identical text only
// updates the hash, so echoing the same clipboard back and forth does not bump
// the revision forever.
func (s *Store) Set(text string) uint64 {
	return s.set(text, false)
}

// SetFromRemote stores text that came from the phone: the revision changes even
// when the text repeats, because the PC clipboard is about to be written.
func (s *Store) SetFromRemote(text string) uint64 {
	return s.set(text, true)
}

func (s *Store) set(text string, force bool) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !force && hashText(text) == s.hash {
		return s.rev
	}
	s.rev++
	s.text = text
	s.hash = hashText(text)
	for waiter := range s.waiters {
		close(waiter)
		delete(s.waiters, waiter)
	}
	return s.rev
}

// Snapshot is the current revision and text.
func (s *Store) Snapshot() (uint64, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rev, s.text
}

// Hash is the hash of the text that last entered the store, used to ignore the
// clipboard change the PC itself just made.
func (s *Store) Hash() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hash
}

// Wait blocks until the revision passes since, or the timeout expires.
func (s *Store) Wait(ctx context.Context, since uint64, timeout time.Duration) (uint64, string) {
	s.mu.Lock()
	if s.rev > since {
		rev, text := s.rev, s.text
		s.mu.Unlock()
		return rev, text
	}
	waiter := make(chan struct{})
	if s.waiters == nil {
		s.waiters = map[chan struct{}]struct{}{}
	}
	s.waiters[waiter] = struct{}{}
	s.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waiter:
	case <-timer.C:
	case <-ctx.Done():
	}

	s.mu.Lock()
	delete(s.waiters, waiter)
	rev, text := s.rev, s.text
	s.mu.Unlock()
	return rev, text
}

// Server is the HTTP endpoint the phone talks to.
type Server struct {
	token    string
	store    *Store
	onRemote func(string)
	http     *http.Server
	addr     string
}

// NewServer builds a server with a token and a callback that applies text from
// the phone to the PC clipboard.
func NewServer(token string, store *Store, onRemote func(string)) *Server {
	s := &Server{token: token, store: store, onRemote: onRemote}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/clip", s.handleClip)
	mux.HandleFunc("/api/ping", s.handlePing)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

// Start listens on every interface so the phone can reach it, and returns the
// address that was opened.
func (s *Server) Start() (string, error) {
	listener, err := net.Listen("tcp4", ":47820")
	if err != nil {
		return "", err
	}
	s.addr = listener.Addr().String()
	go func() { _ = s.http.Serve(listener) }()
	return s.addr, nil
}

// Close stops the server.
func (s *Server) Close() error {
	if s.http == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}

func (s *Server) authorized(r *http.Request) bool {
	token := r.URL.Query().Get("t")
	if token == "" {
		token = r.Header.Get("X-Token")
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rev, _ := s.store.Snapshot()
	writeJSON(w, map[string]any{"ok": true, "rev": rev})
}

func (s *Server) handleClip(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
		rev, text := s.store.Wait(r.Context(), since, 20*time.Second)
		writeJSON(w, map[string]any{"rev": rev, "text": text})
	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, maxText+1))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if len(body) > maxText {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
			return
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err = json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		rev := s.store.SetFromRemote(payload.Text)
		if s.onRemote != nil {
			s.onRemote(payload.Text)
		}
		writeJSON(w, map[string]any{"rev": rev})
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		// The client went away; nothing sensible to report.
		_ = fmt.Sprint(err)
	}
}

// Pairing is the one line the phone needs: address and token.
func Pairing(addr, token string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		host, port = addr, "47820"
	}
	return net.JoinHostPort(host, port) + "#" + token
}
