package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	apptoken "telegram-message-forward/internal/app/apitoken"
	appauth "telegram-message-forward/internal/app/auth"
	domainadmin "telegram-message-forward/internal/domain/admin"
	"telegram-message-forward/internal/security"
)

const (
	passwordTestOld = "old-test-password-123"
	passwordTestNew = "new-test-password-456789"
)

func passwordAuthToken(t *testing.T, router http.Handler, path, password string) string {
	t.Helper()
	code, body := do(t, router, http.MethodPost, path, "", map[string]string{"username": "admin", "password": password})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("test login failed: status=%d", code)
	}
	var response struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil || response.Data.Token == "" {
		t.Fatal("test login did not return a token")
	}
	return response.Data.Token
}

func TestPasswordChangeRevokesAllSessions(t *testing.T) {
	tokens := newFakeTokenRepo()
	router := buildRouter(t, true, tokens)
	first := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
	second := passwordAuthToken(t, router, "/api/v1/auth/login", passwordTestOld)
	apiToken := "separate-test-api-token"
	if _, err := tokens.Create(context.Background(), "automation", security.HashToken(apiToken)); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, router, http.MethodPost, "/api/v1/auth/password", first, map[string]string{
		"current_password": passwordTestOld, "new_password": passwordTestNew,
	})
	if code != http.StatusNoContent || body != "" {
		t.Fatalf("password change should return an empty 204 response: status=%d", code)
	}
	for _, token := range []string{first, second} {
		if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusUnauthorized {
			t.Fatalf("all old sessions must be rejected: status=%d", code)
		}
	}
	if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"username": "admin", "password": passwordTestOld}); code != http.StatusUnauthorized {
		t.Fatalf("old password must be rejected: status=%d", code)
	}
	fresh := passwordAuthToken(t, router, "/api/v1/auth/login", passwordTestNew)
	for _, token := range []string{fresh, apiToken} {
		if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusOK {
			t.Fatalf("new login and independent API token must remain usable: status=%d", code)
		}
	}
}

func TestPasswordChangeInvalidInputPreservesAccount(t *testing.T) {
	for _, tc := range []struct {
		name, current, next string
	}{
		{"wrong current password", "incorrect-password", passwordTestNew},
		{"missing current password", "", passwordTestNew},
		{"missing new password", passwordTestOld, ""},
		{"short password", passwordTestOld, "short"},
		{"too many bytes", passwordTestOld, strings.Repeat("a", 73)},
		{"unicode byte limit", passwordTestOld, strings.Repeat("密", 25)},
		{"unicode character minimum", passwordTestOld, strings.Repeat("密", 15)},
		{"blank password", passwordTestOld, strings.Repeat(" ", 16)},
		{"unchanged password", passwordTestOld, passwordTestOld},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := buildRouter(t, true, newFakeTokenRepo())
			token := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
			code, body := do(t, router, http.MethodPost, "/api/v1/auth/password", token, map[string]string{
				"current_password": tc.current, "new_password": tc.next,
			})
			if code != http.StatusBadRequest {
				t.Fatalf("invalid password input must return 400: status=%d", code)
			}
			for _, secret := range []string{tc.current, tc.next} {
				if secret != "" && strings.Contains(body, secret) {
					t.Fatal("password must never appear in an error response")
				}
			}
			if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusOK {
				t.Fatalf("failed password change must keep the session valid: status=%d", code)
			}
			passwordAuthToken(t, router, "/api/v1/auth/login", passwordTestOld)
		})
	}
}

func TestPasswordChangeRequiresBrowserSessionEvenWithoutAuth(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		tokens := newFakeTokenRepo()
		apiToken := "test-automation-token"
		_, _ = tokens.Create(context.Background(), "automation", security.HashToken(apiToken))
		router := buildRouter(t, enabled, tokens)
		session := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
		if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/logout", session, nil); code != http.StatusNoContent {
			t.Fatal("test logout failed")
		}
		for _, token := range []string{"", "invalid-token", apiToken, session} {
			if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/password", token, map[string]string{
				"current_password": passwordTestOld, "new_password": passwordTestNew,
			}); code != http.StatusUnauthorized {
				t.Fatalf("missing/invalid/revoked session or API token must not change passwords (auth=%t): status=%d", enabled, code)
			}
		}
	}
}

func TestPasswordChangeAcceptsLongAndUnicodePasswords(t *testing.T) {
	for _, password := range []string{strings.Repeat("a", 72), strings.Repeat("密", 16), "  long password with spaces  "} {
		router := buildRouter(t, true, newFakeTokenRepo())
		token := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
		if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/password", token, map[string]string{
			"current_password": passwordTestOld, "new_password": password,
		}); code != http.StatusNoContent {
			t.Fatalf("valid long password must be accepted: status=%d", code)
		}
		passwordAuthToken(t, router, "/api/v1/auth/login", password)
	}
}

type passwordChangeBeforeLoginRepo struct {
	*fakeAdminRepo
	beforeCreate func()
}

func (r *passwordChangeBeforeLoginRepo) CreateSession(ctx context.Context, sess *domainadmin.Session, passwordHash string) error {
	if r.beforeCreate != nil {
		r.beforeCreate()
	}
	return r.fakeAdminRepo.CreateSession(ctx, sess, passwordHash)
}

func TestPasswordChangePreventsStaleLoginFromIssuingSession(t *testing.T) {
	ctx := context.Background()
	admins := &passwordChangeBeforeLoginRepo{fakeAdminRepo: newFakeAdminRepo()}
	tokens := newFakeTokenRepo()
	svc := appauth.NewService(admins, tokens, apptoken.NewService(tokens), true)
	if _, err := svc.Bootstrap(ctx, "admin", passwordTestOld); err != nil {
		t.Fatal(err)
	}
	admins.beforeCreate = func() {
		user, _ := admins.GetUserByUsername(ctx, "admin")
		hash, err := bcrypt.GenerateFromPassword([]byte(passwordTestNew), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		if changed, err := admins.ChangePassword(ctx, user.ID, user.PasswordHash, string(hash)); err != nil || !changed {
			t.Fatal("concurrent password change failed")
		}
	}
	if _, err := svc.Login(ctx, "admin", passwordTestOld); !errors.Is(err, domainadmin.ErrCredentialChanged) {
		t.Fatalf("old password validated before a change must not issue a new session: %v", err)
	}
	for _, session := range admins.sessions {
		if session.RevokedAt == nil && session.ExpiresAt.After(time.Now()) {
			t.Fatal("stale login left an active session after password change")
		}
	}
}
