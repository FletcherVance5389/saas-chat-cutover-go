package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
)

type server struct {
	chat *TenantChat
}

type onboardInput struct {
	AccountID string `json:"account_id"`
	ClientID  string `json:"client_id"`
	RequestID string `json:"request_id"`
}

type stateInput struct {
	State AccountState `json:"state"`
}

type announcementInput struct {
	Event     string `json:"event"`
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	s := &server{chat: NewTenantChat(NewInfraiClient(key))}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tenants/onboard", s.onboard)
	mux.HandleFunc("PUT /accounts/{accountID}/state", s.setState)
	mux.HandleFunc("POST /admin/accounts/{accountID}/announcements", s.announce)
	log.Println("chat room service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func (s *server) onboard(w http.ResponseWriter, r *http.Request) {
	var in onboardInput
	if !decode(w, r, &in) {
		return
	}
	if in.AccountID == "" || in.ClientID == "" || in.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account_id, client_id, and request_id are required"})
		return
	}
	result, err := s.chat.Onboard(r.Context(), in.AccountID, in.ClientID, in.RequestID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *server) setState(w http.ResponseWriter, r *http.Request) {
	var in stateInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.chat.SetAccountState(r.PathValue("accountID"), in.State); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account_id": r.PathValue("accountID"), "state": in.State})
}

func (s *server) announce(w http.ResponseWriter, r *http.Request) {
	var in announcementInput
	if !decode(w, r, &in) {
		return
	}
	if in.Event == "" || in.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event and request_id are required"})
		return
	}
	result, err := s.chat.Announce(r.Context(), r.PathValue("accountID"), in.Event, in.Data, in.RequestID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return false
	}
	return true
}

func writeServiceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccountSuspended) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrAccountNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "realtime request failed"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !strings.Contains(err.Error(), "closed") {
		log.Printf("encode response: %v", err)
	}
}
