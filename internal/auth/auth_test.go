package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDeviceLoginPollingSuccessPersistsSession(t *testing.T) {
	var pollCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/cli-auth/device":
			writeJSON(t, w, http.StatusCreated, DeviceFlowResponse{
				DeviceCode:              "device-123",
				UserCode:                "ABCD-EFGH",
				VerificationURI:         serverURL(r) + "/auth/cli",
				VerificationURIComplete: serverURL(r) + "/auth/cli?user_code=ABCD-EFGH",
				ExpiresIn:               600,
				Interval:                1,
			})
		case "/api/cli-auth/token":
			pollCount++
			if pollCount == 1 {
				writeJSON(t, w, http.StatusPreconditionRequired, TokenError{
					Error:            "authorization_pending",
					ErrorDescription: "Authorization is still pending.",
					Interval:         1,
				})
				return
			}
			writeJSON(t, w, http.StatusOK, TokenSuccess{
				AccessToken: "token-123",
				TokenType:   "bearer",
				RegistryURL: "https://registry.example.com",
				User: User{
					ID:    "user_123",
					Email: "founder@example.com",
					Name:  "Founder",
				},
				Plan: "pro",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	storePath := filepath.Join(t.TempDir(), "auth.json")
	service := NewService(FileStore{Path: storePath}, server.URL, server.Client())
	service.OpenBrowser = func(string) error { return nil }
	service.Sleep = func(context.Context, time.Duration) error { return nil }
	service.Hostname = func() (string, error) { return "mbp.local", nil }
	service.EnvToken = func() string { return "" }

	started, err := service.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin returned error: %v", err)
	}
	if started.UserCode != "ABCD-EFGH" {
		t.Fatalf("expected user code, got %+v", started)
	}

	result, err := service.WaitForLogin(context.Background(), started)
	if err != nil {
		t.Fatalf("WaitForLogin returned error: %v", err)
	}
	if result.RegistryURL != "https://registry.example.com" {
		t.Fatalf("expected registry url, got %+v", result)
	}

	stored, err := service.Store.Read()
	if err != nil {
		t.Fatalf("Store.Read returned error: %v", err)
	}
	if stored.AccessToken != "token-123" || stored.User.Email != "founder@example.com" {
		t.Fatalf("stored session mismatch: %+v", stored)
	}
	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 auth file, got %#o", info.Mode().Perm())
	}
}

func TestLoginHandlesPendingAndSlowDown(t *testing.T) {
	var polls int
	var sleeps []time.Duration

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/cli-auth/device":
			writeJSON(t, w, http.StatusCreated, DeviceFlowResponse{
				DeviceCode:              "device-123",
				UserCode:                "ABCD-EFGH",
				VerificationURI:         serverURL(r) + "/auth/cli",
				VerificationURIComplete: serverURL(r) + "/auth/cli?user_code=ABCD-EFGH",
				ExpiresIn:               600,
				Interval:                1,
			})
		case "/api/cli-auth/token":
			polls++
			switch polls {
			case 1:
				writeJSON(t, w, http.StatusPreconditionRequired, TokenError{
					Error:            "authorization_pending",
					ErrorDescription: "Authorization is still pending.",
					Interval:         2,
				})
			case 2:
				writeJSON(t, w, http.StatusTooManyRequests, TokenError{
					Error:            "slow_down",
					ErrorDescription: "Poll less frequently.",
					Interval:         7,
				})
			default:
				writeJSON(t, w, http.StatusOK, TokenSuccess{
					AccessToken: "token-123",
					TokenType:   "bearer",
					RegistryURL: "https://registry.example.com",
					User: User{
						ID:    "user_123",
						Email: "founder@example.com",
					},
					Plan: "free",
				})
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewService(FileStore{Path: filepath.Join(t.TempDir(), "auth.json")}, server.URL, server.Client())
	service.OpenBrowser = func(string) error { return nil }
	service.Sleep = func(_ context.Context, duration time.Duration) error {
		sleeps = append(sleeps, duration)
		return nil
	}

	started, err := service.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin returned error: %v", err)
	}
	if _, err := service.WaitForLogin(context.Background(), started); err != nil {
		t.Fatalf("WaitForLogin returned error: %v", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 7 * time.Second}
	if !slices.Equal(sleeps, want) {
		t.Fatalf("sleep durations mismatch: got %v want %v", sleeps, want)
	}
}

func TestLoginHandlesExpiredDeniedAndInvalidRequest(t *testing.T) {
	tests := []struct {
		name        string
		tokenError  TokenError
		statusCode  int
		expectMatch string
	}{
		{
			name: "expired",
			tokenError: TokenError{
				Error:            "expired_token",
				ErrorDescription: "The device authorization request has expired.",
			},
			statusCode:  http.StatusGone,
			expectMatch: "expired",
		},
		{
			name: "denied",
			tokenError: TokenError{
				Error:            "access_denied",
				ErrorDescription: "The authorization request was denied.",
			},
			statusCode:  http.StatusForbidden,
			expectMatch: "denied",
		},
		{
			name: "invalid",
			tokenError: TokenError{
				Error:            "invalid_request",
				ErrorDescription: "The device code is invalid.",
			},
			statusCode:  http.StatusBadRequest,
			expectMatch: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/cli-auth/device":
					writeJSON(t, w, http.StatusCreated, DeviceFlowResponse{
						DeviceCode:              "device-123",
						UserCode:                "ABCD-EFGH",
						VerificationURI:         serverURL(r) + "/auth/cli",
						VerificationURIComplete: serverURL(r) + "/auth/cli?user_code=ABCD-EFGH",
						ExpiresIn:               600,
						Interval:                1,
					})
				case "/api/cli-auth/token":
					writeJSON(t, w, tt.statusCode, tt.tokenError)
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()

			service := NewService(FileStore{Path: filepath.Join(t.TempDir(), "auth.json")}, server.URL, server.Client())
			service.OpenBrowser = func(string) error { return nil }
			service.Sleep = func(context.Context, time.Duration) error { return nil }

			started, err := service.StartLogin(context.Background())
			if err != nil {
				t.Fatalf("StartLogin returned error: %v", err)
			}
			if _, err := service.WaitForLogin(context.Background(), started); err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.expectMatch) {
				t.Fatalf("expected error containing %q, got %v", tt.expectMatch, err)
			}
		})
	}
}

func TestStatusReportsValidAndInvalidStoredToken(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/cli-auth/session" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			if r.Header.Get("authorization") != "Bearer token-123" {
				t.Fatalf("unexpected authorization header %q", r.Header.Get("authorization"))
			}
			writeJSON(t, w, http.StatusOK, SessionInfo{
				RegistryURL: "https://registry.example.com",
				User: User{
					ID:    "user_123",
					Email: "founder@example.com",
				},
				Plan: "pro",
			})
		}))
		defer server.Close()

		store := FileStore{Path: filepath.Join(t.TempDir(), "auth.json")}
		if err := store.Write(Session{
			AccessToken: "token-123",
			RegistryURL: "https://registry.example.com",
			AuthURL:     server.URL,
			User:        User{Email: "founder@example.com"},
			Plan:        "pro",
		}); err != nil {
			t.Fatalf("store.Write returned error: %v", err)
		}

		service := NewService(store, server.URL, server.Client())
		service.EnvToken = func() string { return "" }

		result, err := service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status returned error: %v", err)
		}
		if !result.SignedIn || result.User.Email != "founder@example.com" {
			t.Fatalf("unexpected status result: %+v", result)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		store := FileStore{Path: filepath.Join(t.TempDir(), "auth.json")}
		if err := store.Write(Session{
			AccessToken: "token-123",
			RegistryURL: "https://registry.example.com",
			AuthURL:     server.URL,
			User:        User{Email: "founder@example.com"},
			Plan:        "pro",
		}); err != nil {
			t.Fatalf("store.Write returned error: %v", err)
		}

		service := NewService(store, server.URL, server.Client())
		service.EnvToken = func() string { return "" }

		result, err := service.Status(context.Background())
		if err != nil {
			t.Fatalf("Status returned error: %v", err)
		}
		if result.SignedIn || !result.InvalidToken || result.TokenSource != TokenSourceStored {
			t.Fatalf("unexpected invalid status result: %+v", result)
		}
	})
}

func TestLogoutClearsLocalStateOnUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli-auth/session" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	storePath := filepath.Join(t.TempDir(), "auth.json")
	store := FileStore{Path: storePath}
	if err := store.Write(Session{
		AccessToken: "token-123",
		RegistryURL: "https://registry.example.com",
		AuthURL:     server.URL,
		User:        User{Email: "founder@example.com"},
		Plan:        "free",
	}); err != nil {
		t.Fatalf("store.Write returned error: %v", err)
	}

	service := NewService(store, server.URL, server.Client())
	result, err := service.Logout(context.Background())
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if !result.ClearedSession || !result.LoggedOut {
		t.Fatalf("unexpected logout result: %+v", result)
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("expected local auth file to be removed, got %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
