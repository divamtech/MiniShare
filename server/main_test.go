package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupTestHandler creates a fresh Store and returns an http.Handler with all
// routes registered (matching main.go's setup). Uses httptest.ResponseRecorder
// to avoid network port binding.
func setupTestHandler() http.Handler {
	store := NewStore()
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
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

		sess := &Session{UUID: uuid, Offer: req.Offer}
		store.mu.Lock()
		store.sessions[uuid] = sess
		store.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"uuid": uuid})
	})

	mux.HandleFunc("/api/session/", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
			return
		}
		if r.URL.Path == "/app" || strings.HasPrefix(r.URL.Path, "/app/") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(appHTML)
			return
		}
		http.NotFound(w, r)
	})

	return mux
}

// doRequest is a helper that makes an HTTP request via httptest.ResponseRecorder
func doRequest(handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Health Endpoint
// ---------------------------------------------------------------------------
func TestHealthEndpoint(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/health", "")

	if rr.Code != http.StatusOK {
		t.Errorf("GET /health status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("GET /health body = %q, want 'OK'", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Session Creation (POST /api/session)
// ---------------------------------------------------------------------------
func TestCreateSession(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodPost, "/api/session", `{"offer":"test-offer-data","uuid":"test-session-123"}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/session status = %d, want %d", rr.Code, http.StatusOK)
	}

	var result map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if result["uuid"] != "test-session-123" {
		t.Errorf("uuid = %q, want 'test-session-123'", result["uuid"])
	}
}

func TestCreateSession_GeneratesUUID(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodPost, "/api/session", `{"offer":"test-offer-data"}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/session status = %d, want %d", rr.Code, http.StatusOK)
	}

	var result map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&result)
	if result["uuid"] == "" {
		t.Error("Expected auto-generated UUID, got empty string")
	}
}

func TestCreateSession_MissingOffer(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodPost, "/api/session", `{"uuid":"test-123"}`)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("POST /api/session without offer: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateSession_InvalidJSON(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodPost, "/api/session", "{invalid json")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("POST /api/session with invalid JSON: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCreateSession_MethodNotAllowed(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/api/session", "")

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/session status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Get Offer (GET /api/session/{uuid}/offer)
// ---------------------------------------------------------------------------
func TestGetOffer(t *testing.T) {
	handler := setupTestHandler()

	// Create session first
	doRequest(handler, http.MethodPost, "/api/session", `{"offer":"my-offer-sdp","uuid":"offer-test-1"}`)

	// Get offer
	rr := doRequest(handler, http.MethodGet, "/api/session/offer-test-1/offer", "")

	if rr.Code != http.StatusOK {
		t.Fatalf("GET offer status = %d, want %d", rr.Code, http.StatusOK)
	}

	var result map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&result)
	if result["offer"] != "my-offer-sdp" {
		t.Errorf("offer = %q, want 'my-offer-sdp'", result["offer"])
	}
}

func TestGetOffer_NotFound(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/api/session/nonexistent/offer", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET offer for missing session: status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Answer (POST + GET /api/session/{uuid}/answer)
// ---------------------------------------------------------------------------
func TestPostAndGetAnswer(t *testing.T) {
	handler := setupTestHandler()

	// Create session
	doRequest(handler, http.MethodPost, "/api/session", `{"offer":"offer-data","uuid":"answer-test-1"}`)

	// Post answer
	rr := doRequest(handler, http.MethodPost, "/api/session/answer-test-1/answer", `{"answer":"my-answer-sdp"}`)
	if rr.Code != http.StatusOK {
		t.Errorf("POST answer status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Get answer
	rr2 := doRequest(handler, http.MethodGet, "/api/session/answer-test-1/answer", "")
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET answer status = %d, want %d", rr2.Code, http.StatusOK)
	}

	var result map[string]string
	_ = json.NewDecoder(rr2.Body).Decode(&result)
	if result["answer"] != "my-answer-sdp" {
		t.Errorf("answer = %q, want 'my-answer-sdp'", result["answer"])
	}
}

func TestPostAnswer_MissingAnswer(t *testing.T) {
	handler := setupTestHandler()

	// Create session
	doRequest(handler, http.MethodPost, "/api/session", `{"offer":"offer-data","uuid":"missing-answer-test"}`)

	// Post empty answer
	rr := doRequest(handler, http.MethodPost, "/api/session/missing-answer-test/answer", `{}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("POST answer without answer field: status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetAnswer_EmptyBeforeSet(t *testing.T) {
	handler := setupTestHandler()

	// Create session
	doRequest(handler, http.MethodPost, "/api/session", `{"offer":"offer-data","uuid":"empty-answer-test"}`)

	// Get answer before it's set
	rr := doRequest(handler, http.MethodGet, "/api/session/empty-answer-test/answer", "")

	var result map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&result)
	if result["answer"] != "" {
		t.Errorf("answer before set = %q, want empty", result["answer"])
	}
}

// ---------------------------------------------------------------------------
// Static Pages (GET / and GET /app)
// ---------------------------------------------------------------------------
func TestRootPage(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/", "")

	if rr.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", rr.Code, http.StatusOK)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "MiniShare") {
		t.Error("GET / body doesn't contain 'MiniShare'")
	}
}

func TestAppPage(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/app", "")

	if rr.Code != http.StatusOK {
		t.Errorf("GET /app status = %d, want %d", rr.Code, http.StatusOK)
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("GET /app Content-Type = %q, want text/html", ct)
	}
}

func TestAppPage_WithUUID(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/app/some-uuid-here", "")

	if rr.Code != http.StatusOK {
		t.Errorf("GET /app/uuid status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestNotFoundPage(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/nonexistent", "")

	if rr.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Invalid Session Action
// ---------------------------------------------------------------------------
func TestInvalidAction(t *testing.T) {
	handler := setupTestHandler()

	// Create session
	doRequest(handler, http.MethodPost, "/api/session", `{"offer":"offer-data","uuid":"action-test"}`)

	rr := doRequest(handler, http.MethodGet, "/api/session/action-test/invalid", "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("GET invalid action: status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Store and generateUUID
// ---------------------------------------------------------------------------
func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatal("NewStore() returned nil")
	}
	if store.sessions == nil {
		t.Fatal("NewStore().sessions is nil")
	}
}

func TestGenerateUUID_Server(t *testing.T) {
	uuid := generateUUID()
	if uuid == "" {
		t.Fatal("generateUUID() returned empty string")
	}
	if strings.Count(uuid, "-") != 4 {
		t.Errorf("generateUUID() = %q, expected 4 dashes", uuid)
	}
}

// ---------------------------------------------------------------------------
// Full Flow: Create → Get Offer → Post Answer → Get Answer
// ---------------------------------------------------------------------------
func TestFullSignalingFlow(t *testing.T) {
	handler := setupTestHandler()

	// 1. Host creates session
	createRR := doRequest(handler, http.MethodPost, "/api/session", `{"offer":"host-offer-sdp","uuid":"flow-test-123"}`)
	if createRR.Code != http.StatusOK {
		t.Fatalf("Create session status = %d", createRR.Code)
	}

	// 2. Viewer fetches offer
	offerRR := doRequest(handler, http.MethodGet, "/api/session/flow-test-123/offer", "")
	var offerData map[string]string
	_ = json.NewDecoder(offerRR.Body).Decode(&offerData)
	if offerData["offer"] != "host-offer-sdp" {
		t.Errorf("Offer = %q, want 'host-offer-sdp'", offerData["offer"])
	}

	// 3. Viewer posts answer
	answerRR := doRequest(handler, http.MethodPost, "/api/session/flow-test-123/answer", `{"answer":"viewer-answer-sdp"}`)
	if answerRR.Code != http.StatusOK {
		t.Fatalf("Post answer status = %d", answerRR.Code)
	}

	// 4. Host fetches answer
	getAnswerRR := doRequest(handler, http.MethodGet, "/api/session/flow-test-123/answer", "")
	var answerData map[string]string
	_ = json.NewDecoder(getAnswerRR.Body).Decode(&answerData)
	if answerData["answer"] != "viewer-answer-sdp" {
		t.Errorf("Answer = %q, want 'viewer-answer-sdp'", answerData["answer"])
	}
}

// ---------------------------------------------------------------------------
// Landing page includes platform download section (Bug 4 & 5 verification)
// ---------------------------------------------------------------------------
func TestLandingPage_ContainsPlatformDownloads(t *testing.T) {
	handler := setupTestHandler()
	rr := doRequest(handler, http.MethodGet, "/", "")

	body := rr.Body.String()
	checks := []string{
		"Download &amp; Install",
		"Apple Silicon",
		"Intel (x86_64)",
		"Linux",
		"Windows",
		"releases/latest",
		"View All Releases",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("Landing page missing expected content: %q", check)
		}
	}
}
