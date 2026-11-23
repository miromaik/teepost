package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"teepost/internal/db"
)

const (
	sessionCookieName    = "teepost_session"
	adminPasswordHashKey = "admin_password_hash"
	cookieSecretKey      = "auth_cookie_secret"
)

type authManager struct {
	store  *db.Store
	secret []byte
}

func newAuthManager(store *db.Store, envPassword string) *authManager {
	secretHex, err := store.GetSetting(cookieSecretKey)
	if err != nil || len(secretHex) < 32 {
		buf := make([]byte, 32)
		if _, rErr := rand.Read(buf); rErr != nil {
			sum := sha256.Sum256([]byte("teepost-fallback-secret"))
			buf = sum[:]
		}
		secretHex = hex.EncodeToString(buf)
		_ = store.SetSetting(cookieSecretKey, secretHex)
	}
	secret, err := hex.DecodeString(secretHex)
	if err != nil || len(secret) == 0 {
		sum := sha256.Sum256([]byte(secretHex))
		secret = sum[:]
	}

	// If ADMIN_PASSWORD env var was provided and no password is stored in SQLite yet, seed it.
	if envPassword != "" {
		if _, err := store.GetSetting(adminPasswordHashKey); errors.Is(err, db.ErrNotFound) {
			_ = store.SetSetting(adminPasswordHashKey, hashPassword(envPassword))
		}
	}

	return &authManager{
		store:  store,
		secret: secret,
	}
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte("teepost-pw-v1:" + password))
	return hex.EncodeToString(sum[:])
}

func (a *authManager) passwordHash() string {
	val, err := a.store.GetSetting(adminPasswordHashKey)
	if err != nil {
		return ""
	}
	return val
}

func (a *authManager) isEnabled() bool {
	return a.passwordHash() != ""
}

func (a *authManager) sessionToken(pwHash string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte("teepost-auth:" + pwHash))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *authManager) isRequestAuthenticated(r *http.Request) bool {
	pwHash := a.passwordHash()
	if pwHash == "" {
		return true
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	expected := a.sessionToken(pwHash)
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(expected)) == 1
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func (a *authManager) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.isEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/healthz" || r.URL.Path == "/ready" || r.URL.Path == "/api/health" || r.URL.Path == "/api/auth/status" || r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && !a.isRequestAuthenticated(r) {
			writeJSONError(w, http.StatusUnauthorized, "Authentication required.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authRequired":  s.auth.isEnabled(),
		"authenticated": s.auth.isRequestAuthenticated(r),
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	pwHash := s.auth.passwordHash()
	if pwHash == "" {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload.")
		return
	}

	candidateHash := hashPassword(req.Password)
	if subtle.ConstantTimeCompare([]byte(candidateHash), []byte(pwHash)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "Invalid password.")
		return
	}

	setSessionCookie(w, r, s.auth.sessionToken(pwHash), 86400*30)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	setSessionCookie(w, r, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (s *Server) handleSetAuthPassword(w http.ResponseWriter, r *http.Request) {
	if s.auth.isEnabled() && !s.auth.isRequestAuthenticated(r) {
		writeJSONError(w, http.StatusUnauthorized, "Authentication required.")
		return
	}

	var req struct {
		Enabled  *bool  `json:"enabled"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload.")
		return
	}

	disable := (req.Enabled != nil && !*req.Enabled) || strings.TrimSpace(req.Password) == ""
	if disable {
		if err := s.store.DeleteSetting(adminPasswordHashKey); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		setSessionCookie(w, r, "", -1)
		writeJSON(w, http.StatusOK, map[string]any{
			"authRequired":  false,
			"authenticated": true,
		})
		return
	}

	newHash := hashPassword(req.Password)
	if err := s.store.SetSetting(adminPasswordHashKey, newHash); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	setSessionCookie(w, r, s.auth.sessionToken(newHash), 86400*30)
	writeJSON(w, http.StatusOK, map[string]any{
		"authRequired":  true,
		"authenticated": true,
	})
}
