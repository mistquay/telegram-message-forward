package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	apptoken "telegram-message-forward/internal/app/apitoken"
	appauth "telegram-message-forward/internal/app/auth"
	domainadmin "telegram-message-forward/internal/domain/admin"
	"telegram-message-forward/internal/security"
)

func TestUsernameChangeRequiresNewNameAndRevokesAllSessions(t *testing.T) {
	tokens := newFakeTokenRepo()
	router := buildRouter(t, true, tokens)
	first := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
	second := passwordAuthToken(t, router, "/api/v1/auth/login", passwordTestOld)
	apiToken := "separate-username-test-api-token"
	_, _ = tokens.Create(context.Background(), "automation", security.HashToken(apiToken))
	code, body := do(t, router, http.MethodPost, "/api/v1/auth/username", first, map[string]string{
		"current_password": passwordTestOld, "new_username": "  custom-owner  ",
	})
	if code != http.StatusNoContent || body != "" {
		t.Fatalf("username change must return an empty 204 response: status=%d", code)
	}
	for _, token := range []string{first, second} {
		if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusUnauthorized {
			t.Fatalf("old sessions must be revoked: status=%d", code)
		}
	}
	if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"username": "admin", "password": passwordTestOld}); code != http.StatusUnauthorized {
		t.Fatalf("old username must no longer log in: status=%d", code)
	}
	fresh := accountAuthToken(t, router, "/api/v1/auth/login", "custom-owner", passwordTestOld)
	for _, token := range []string{fresh, apiToken} {
		if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusOK {
			t.Fatalf("new name login and independent API token must work: status=%d", code)
		}
	}
	if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/password", fresh, map[string]string{
		"current_password": passwordTestOld, "new_password": passwordTestNew,
	}); code != http.StatusNoContent {
		t.Fatalf("renamed account must still be able to change its password: status=%d", code)
	}
	accountAuthToken(t, router, "/api/v1/auth/login", "custom-owner", passwordTestNew)
}

func TestUsernameChangeInvalidInputPreservesAccount(t *testing.T) {
	for _, tc := range []struct{ name, current, next string }{
		{"wrong password", "incorrect-test-password", "new-owner"},
		{"missing password", "", "new-owner"},
		{"empty username", passwordTestOld, ""},
		{"too short", passwordTestOld, "ab"},
		{"too long", passwordTestOld, strings.Repeat("a", 65)},
		{"blank username", passwordTestOld, "   "},
		{"internal space", passwordTestOld, "new owner"},
		{"control character", passwordTestOld, "new\x00owner"},
		{"internal newline", passwordTestOld, "new\nowner"},
		{"unchanged", passwordTestOld, " admin "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := buildRouter(t, true, newFakeTokenRepo())
			token := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
			code, body := do(t, router, http.MethodPost, "/api/v1/auth/username", token, map[string]string{
				"current_password": tc.current, "new_username": tc.next,
			})
			if code != http.StatusBadRequest {
				t.Fatalf("invalid username change must return 400: status=%d", code)
			}
			if tc.current != "" && strings.Contains(body, tc.current) {
				t.Fatal("password must not appear in an error response")
			}
			if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusOK {
				t.Fatal("failed username change must preserve the login session")
			}
			passwordAuthToken(t, router, "/api/v1/auth/login", passwordTestOld)
		})
	}
}

func TestUsernameChangeRequiresBrowserSessionEvenWithoutAuth(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		tokens := newFakeTokenRepo()
		apiToken := "username-change-test-api-token"
		_, _ = tokens.Create(context.Background(), "automation", security.HashToken(apiToken))
		router := buildRouter(t, enabled, tokens)
		session := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
		do(t, router, http.MethodPost, "/api/v1/auth/logout", session, nil)
		for _, token := range []string{"", "invalid", apiToken, session} {
			if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/username", token, map[string]string{
				"current_password": passwordTestOld, "new_username": "new-owner",
			}); code != http.StatusUnauthorized {
				t.Fatalf("username change requires a browser session (auth=%t): status=%d", enabled, code)
			}
		}
	}
}

func TestUsernameChangeConflictPreservesBothAccounts(t *testing.T) {
	admins := newFakeAdminRepo()
	router := buildRouterWithRepos(t, true, newFakeTokenRepo(), admins, "")
	token := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
	admins.users[2] = &domainadmin.User{ID: 2, Username: "occupied", Active: true}
	code, _ := do(t, router, http.MethodPost, "/api/v1/auth/username", token, map[string]string{
		"current_password": passwordTestOld, "new_username": "occupied",
	})
	if code != http.StatusConflict {
		t.Fatalf("duplicate username must return 409: status=%d", code)
	}
	if admins.users[1].Username != "admin" || admins.users[2].Username != "occupied" {
		t.Fatal("conflict must not change either account")
	}
	if code, _ := do(t, router, http.MethodGet, "/api/v1/sinks", token, nil); code != http.StatusOK {
		t.Fatal("conflict must preserve the original session")
	}
}

func TestUsernameChangeAcceptsUnicodeAndMaximumLength(t *testing.T) {
	for _, name := range []string{"自定义管理员", strings.Repeat("用", 64)} {
		router := buildRouter(t, true, newFakeTokenRepo())
		token := passwordAuthToken(t, router, "/api/v1/auth/bootstrap", passwordTestOld)
		if code, _ := do(t, router, http.MethodPost, "/api/v1/auth/username", token, map[string]string{
			"current_password": passwordTestOld, "new_username": name,
		}); code != http.StatusNoContent {
			t.Fatalf("valid Unicode username must be accepted: status=%d", code)
		}
		accountAuthToken(t, router, "/api/v1/auth/login", name, passwordTestOld)
	}
}

func TestUsernameChangePreventsStaleLoginFromIssuingSession(t *testing.T) {
	ctx := context.Background()
	admins := &passwordChangeBeforeLoginRepo{fakeAdminRepo: newFakeAdminRepo()}
	tokens := newFakeTokenRepo()
	svc := appauth.NewService(admins, tokens, apptoken.NewService(tokens), true)
	if _, err := svc.Bootstrap(ctx, "admin", passwordTestOld); err != nil {
		t.Fatal(err)
	}
	admins.beforeCreate = func() {
		user, _ := admins.GetUserByUsername(ctx, "admin")
		if changed, err := admins.ChangeUsername(ctx, user.ID, user.Username, user.PasswordHash, "new-owner"); err != nil || !changed {
			t.Fatal("concurrent username change failed")
		}
	}
	if _, err := svc.Login(ctx, "admin", passwordTestOld); !errors.Is(err, domainadmin.ErrCredentialChanged) {
		t.Fatalf("old username validated before a change must not issue a new session: %v", err)
	}
	for _, sess := range admins.sessions {
		if sess.RevokedAt == nil {
			t.Fatal("stale username login left an active session")
		}
	}
}
