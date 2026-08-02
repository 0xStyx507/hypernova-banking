package main

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/hypernova-banking/api/internal/assistant"
	"github.com/hypernova-banking/api/internal/auth"
)

type chatRequest struct {
	Message string `json:"message"`
}

type assistantHandler struct{ service *assistant.Service }

// registerAssistantRoutes keeps chat authenticated and delegates all tool
// access to the provider-neutral assistant service.
func registerAssistantRoutes(router chi.Router, authService *auth.Service, service *assistant.Service) {
	if authService == nil || service == nil {
		return
	}
	handler := assistantHandler{service: service}
	router.Route("/api/v1/chat", func(router chi.Router) {
		router.Use(ledgerAuthentication(authService))
		router.Post("/messages", handler.message)
	})
}

func (h assistantHandler) message(w http.ResponseWriter, r *http.Request) {
	var request chatRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_chat_request", "invalid chat request")
		return
	}
	response, err := h.service.Chat(r.Context(), bearerToken(r.Header.Get("Authorization")), request.Message)
	if err != nil {
		if errors.Is(err, assistant.ErrInvalidMessage) {
			writeErrorCode(w, http.StatusBadRequest, "invalid_chat_message", "message must contain between 1 and 2000 characters")
			return
		}
		writeErrorCode(w, http.StatusBadGateway, "assistant_unavailable", "assistant temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusOK, response)
}
