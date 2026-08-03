package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/hypernova-banking/api/internal/auth"
)

// updateProfile handles safe profile edits after the authenticated MFA guard.
func (h authHandler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var request updateProfileRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	user, err := h.service.UpdateProfile(ctx, authenticatedUser(r), auth.UpdateProfileInput{FullName: request.FullName}, requestMetadata(r))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidInput) {
			writeErrorCode(w, http.StatusBadRequest, "invalid_profile", "profile data is invalid")
			return
		}
		writeError(w, http.StatusInternalServerError, "profile update failed")
		return
	}
	writeJSON(w, http.StatusOK, userResponseFrom(user))
}
