package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var indexHTML []byte

type Session struct {
	UUID      string    `json:"uuid"`
	Offer     string    `json:"offer,omitempty"`
	Answer    string    `json:"answer,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewStore() *Store {
	s := &Store{sessions: make(map[string]*Session)}
	// Periodic cleanup of expired sessions (24-hour TTL)
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			s.mu.Lock()
			now := time.Now()
			for id, sess := range s.sessions {
				if now.Sub(sess.CreatedAt) > 24*time.Hour {
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		}
	}()
	return s
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func main() {
	port := flag.Int("port", 8080, "Port for signaling server")
	flag.Parse()

	store := NewStore()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			UUID  string `json:"uuid"`
			Offer string `json:"offer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Offer == "" {
			http.Error(w, "Invalid offer payload", http.StatusBadRequest)
			return
		}

		uuid := strings.TrimSpace(req.UUID)
		if uuid == "" {
			uuid = generateUUID()
		}

		sess := &Session{
			UUID:      uuid,
			Offer:     req.Offer,
			CreatedAt: time.Now(),
		}

		store.mu.Lock()
		store.sessions[uuid] = sess
		store.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"uuid": uuid,
		})
	})

	http.HandleFunc("/api/session/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 {
			http.Error(w, "Invalid route", http.StatusNotFound)
			return
		}
		uuid := parts[2]
		action := parts[3]

		store.mu.RLock()
		sess, exists := store.sessions[uuid]
		store.mu.RUnlock()

		if !exists {
			http.Error(w, "Session not found or expired", http.StatusNotFound)
			return
		}

		switch action {
		case "offer":
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"offer": sess.Offer})

		case "answer":
			if r.Method == http.MethodPost {
				var req struct {
					Answer string `json:"answer"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Answer == "" {
					http.Error(w, "Invalid answer payload", http.StatusBadRequest)
					return
				}
				store.mu.Lock()
				sess.Answer = req.Answer
				store.mu.Unlock()
				w.WriteHeader(http.StatusOK)
			} else if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"answer": sess.Answer})
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			http.Error(w, "Invalid action", http.StatusNotFound)
		}
	})

	// Serve Embedded Web SPA at root or /app/
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/app/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("⚡ MiniShare Cloud Signaling Server running at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
