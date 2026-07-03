package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/adhamsalama/inkfeed-backend/db"
	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = 15 * 24 * time.Hour

func validatePassword(password string) error {
	if len(password) < 10 {
		return errors.New("password must be at least 10 characters")
	}
	hasDigit := false
	hasSymbol := false
	for _, c := range password {
		if unicode.IsDigit(c) {
			hasDigit = true
		} else if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			hasSymbol = true
		}
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	if !hasSymbol {
		return errors.New("password must contain at least one symbol")
	}
	return nil
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *App) signupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		jsonError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	if err := validatePassword(req.Password); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := a.q.CreateUser(r.Context(), db.CreateUserParams{
		Email:        req.Email,
		PasswordHash: string(hash),
	})
	if err != nil {
		jsonError(w, "email already registered", http.StatusConflict)
		return
	}

	if err := a.issueSession(w, r, user.ID); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"email": req.Email})
}

func (a *App) signinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		jsonError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	user, err := a.q.GetUserByEmail(r.Context(), req.Email)
	if err == sql.ErrNoRows {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	} else if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := a.issueSession(w, r, user.ID); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"email": user.Email})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

func (a *App) changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CurrentPassword == "" || req.NewPassword == "" || req.ConfirmPassword == "" {
		jsonError(w, "currentPassword, newPassword, and confirmPassword are required", http.StatusBadRequest)
		return
	}

	if req.NewPassword != req.ConfirmPassword {
		jsonError(w, "new passwords do not match", http.StatusBadRequest)
		return
	}

	if err := validatePassword(req.NewPassword); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(contextKey("userID")).(int64)
	user, err := a.q.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		jsonError(w, "current password is incorrect", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := a.q.UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{
		PasswordHash: string(hash),
		ID:           userID,
	}); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "password changed successfully"})
}

func (a *App) issueSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(sessionDuration)

	if err := a.q.CreateSession(r.Context(), db.CreateSessionParams{
		Token:     token,
		UserID:    userID,
		ExpiresAt: expires,
	}); err != nil {
		return err
	}

	secure := strings.HasPrefix(a.allowedOrigins[0], "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
	})
	return nil
}

// authMiddleware validates the session cookie on every request.
func (a *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		session, err := a.q.GetSession(r.Context(), cookie.Value)
		if err == sql.ErrNoRows {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		} else if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), contextKey("userID"), session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
