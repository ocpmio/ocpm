package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Image string `json:"image,omitempty"`
}

type Session struct {
	AccessToken string `json:"accessToken"`
	RegistryURL string `json:"registryUrl"`
	AuthURL     string `json:"authUrl"`
	User        User   `json:"user"`
	Plan        string `json:"plan"`
}

type TokenSource string

const (
	TokenSourceNone   TokenSource = "none"
	TokenSourceEnv    TokenSource = "env"
	TokenSourceStored TokenSource = "stored"
)

type FileStore struct {
	Path string
}

func (s FileStore) Read() (Session, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s FileStore) Write(session Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}

	tempPath := s.Path + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, s.Path)
}

func (s FileStore) Clear() error {
	err := os.Remove(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DeviceFlowResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

type TokenSuccess struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	RegistryURL string `json:"registryUrl"`
	User        User   `json:"user"`
	Plan        string `json:"plan"`
}

type TokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"errorDescription"`
	Interval         int    `json:"interval"`
}

type SessionInfo struct {
	RegistryURL string `json:"registryUrl"`
	User        User   `json:"user"`
	Plan        string `json:"plan"`
}

type HTTPClient struct {
	BaseURL string
	Client  *http.Client
}

func NewHTTPClient(baseURL string, client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  client,
	}
}

func (c *HTTPClient) StartDeviceFlow(ctx context.Context, deviceName string) (DeviceFlowResponse, error) {
	payload := map[string]string{}
	if deviceName != "" {
		payload["deviceName"] = deviceName
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return DeviceFlowResponse{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/cli-auth/device", bytes.NewReader(body))
	if err != nil {
		return DeviceFlowResponse{}, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("accept", "application/json")

	response, err := c.Client.Do(request)
	if err != nil {
		return DeviceFlowResponse{}, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		return DeviceFlowResponse{}, readAPIError(response)
	}

	var result DeviceFlowResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return DeviceFlowResponse{}, err
	}
	return result, nil
}

func (c *HTTPClient) PollToken(ctx context.Context, deviceCode string) (*TokenSuccess, *TokenError, error) {
	body, err := json.Marshal(map[string]string{"deviceCode": deviceCode})
	if err != nil {
		return nil, nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/cli-auth/token", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("accept", "application/json")

	response, err := c.Client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusOK {
		var result TokenSuccess
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			return nil, nil, err
		}
		return &result, nil, nil
	}

	var tokenError TokenError
	if err := json.NewDecoder(response.Body).Decode(&tokenError); err != nil {
		return nil, nil, err
	}
	return nil, &tokenError, nil
}

func (c *HTTPClient) GetSession(ctx context.Context, token string) (*SessionInfo, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/cli-auth/session", nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("authorization", "Bearer "+token)

	response, err := c.Client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return nil, response.StatusCode, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, readAPIError(response)
	}

	var result SessionInfo
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, 0, err
	}
	return &result, response.StatusCode, nil
}

func (c *HTTPClient) DeleteSession(ctx context.Context, token string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+"/api/cli-auth/session", nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("authorization", "Bearer "+token)

	response, err := c.Client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusUnauthorized {
		return response.StatusCode, nil
	}
	return response.StatusCode, readAPIError(response)
}

type Service struct {
	Store       FileStore
	AuthURL     string
	HTTPClient  *http.Client
	OpenBrowser func(string) error
	Hostname    func() (string, error)
	Sleep       func(context.Context, time.Duration) error
	EnvToken    func() string
}

type LoginResult struct {
	UserCode                string
	VerificationURIComplete string
	VerificationURI         string
	DeviceCode              string
	PollIntervalSeconds     int
	RegistryURL             string
	User                    User
	Plan                    string
	BrowserOpened           bool
}

type StatusResult struct {
	SignedIn     bool
	InvalidToken bool
	TokenSource  TokenSource
	RegistryURL  string
	User         User
	Plan         string
}

type LogoutResult struct {
	LoggedOut      bool
	RemoteRevoked  bool
	ClearedSession bool
}

func NewService(store FileStore, authURL string, client *http.Client) *Service {
	return &Service{
		Store:       store,
		AuthURL:     strings.TrimRight(authURL, "/"),
		HTTPClient:  client,
		OpenBrowser: openBrowser,
		Hostname:    os.Hostname,
		Sleep:       sleepWithContext,
		EnvToken: func() string {
			return os.Getenv("OCPM_TOKEN")
		},
	}
}

func (s *Service) Login(ctx context.Context) (LoginResult, error) {
	started, err := s.StartLogin(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	return s.WaitForLogin(ctx, started)
}

func (s *Service) StartLogin(ctx context.Context) (LoginResult, error) {
	client := NewHTTPClient(s.AuthURL, s.HTTPClient)

	deviceName := ""
	if hostname, err := s.Hostname(); err == nil {
		deviceName = strings.TrimSpace(hostname)
	}

	deviceFlow, err := client.StartDeviceFlow(ctx, deviceName)
	if err != nil {
		return LoginResult{}, err
	}

	browserOpened := false
	if err := s.OpenBrowser(deviceFlow.VerificationURIComplete); err == nil {
		browserOpened = true
	}

	return LoginResult{
		UserCode:                deviceFlow.UserCode,
		VerificationURI:         deviceFlow.VerificationURI,
		VerificationURIComplete: deviceFlow.VerificationURIComplete,
		DeviceCode:              deviceFlow.DeviceCode,
		PollIntervalSeconds:     deviceFlow.Interval,
		BrowserOpened:           browserOpened,
	}, nil
}

func (s *Service) WaitForLogin(ctx context.Context, started LoginResult) (LoginResult, error) {
	if started.DeviceCode == "" {
		return LoginResult{}, fmt.Errorf("device authorization was not started")
	}
	if started.UserCode == "" {
		return LoginResult{}, fmt.Errorf("user code is missing")
	}
	if started.VerificationURI == "" {
		started.VerificationURI = started.VerificationURIComplete
	}
	if started.VerificationURIComplete == "" {
		return LoginResult{}, fmt.Errorf("verification URL is missing")
	}
	interval := started.PollIntervalSeconds
	if interval <= 0 {
		interval = 5
	}
	client := NewHTTPClient(s.AuthURL, s.HTTPClient)

	for {
		if err := s.Sleep(ctx, time.Duration(interval)*time.Second); err != nil {
			return LoginResult{}, err
		}

		success, tokenError, err := client.PollToken(ctx, started.DeviceCode)
		if err != nil {
			return LoginResult{}, err
		}
		if success != nil {
			session := Session{
				AccessToken: success.AccessToken,
				RegistryURL: success.RegistryURL,
				AuthURL:     s.AuthURL,
				User:        success.User,
				Plan:        success.Plan,
			}
			if err := s.Store.Write(session); err != nil {
				return LoginResult{}, err
			}
			started.RegistryURL = success.RegistryURL
			started.User = success.User
			started.Plan = success.Plan
			return started, nil
		}

		switch tokenError.Error {
		case "authorization_pending":
			if tokenError.Interval > 0 {
				interval = tokenError.Interval
			}
		case "slow_down":
			if tokenError.Interval > 0 {
				interval = tokenError.Interval
			} else {
				interval += 5
			}
		case "expired_token", "access_denied", "invalid_request":
			return LoginResult{}, errors.New(tokenError.ErrorDescription)
		default:
			return LoginResult{}, fmt.Errorf("unexpected auth polling response: %s", tokenError.Error)
		}
	}
}

func (s *Service) Status(ctx context.Context) (StatusResult, error) {
	stored, err := s.Store.Read()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return StatusResult{}, err
	}

	resolved := ResolveToken(s.EnvToken(), stored)
	if resolved.Source == TokenSourceNone {
		return StatusResult{}, nil
	}

	authURL := s.AuthURL
	if resolved.Source == TokenSourceStored {
		authURL = resolveAuthURL(s.AuthURL, stored.AuthURL)
	}
	client := NewHTTPClient(authURL, s.HTTPClient)
	session, statusCode, err := client.GetSession(ctx, resolved.Token)
	if err != nil {
		return StatusResult{}, err
	}
	if statusCode == http.StatusUnauthorized {
		return StatusResult{
			SignedIn:     false,
			InvalidToken: true,
			TokenSource:  resolved.Source,
			RegistryURL:  stored.RegistryURL,
			User:         stored.User,
			Plan:         stored.Plan,
		}, nil
	}

	return StatusResult{
		SignedIn:    true,
		TokenSource: resolved.Source,
		RegistryURL: session.RegistryURL,
		User:        session.User,
		Plan:        session.Plan,
	}, nil
}

func (s *Service) Logout(ctx context.Context) (LogoutResult, error) {
	stored, err := s.Store.Read()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LogoutResult{ClearedSession: true}, nil
		}
		return LogoutResult{}, err
	}

	result := LogoutResult{ClearedSession: true}
	client := NewHTTPClient(resolveAuthURL(s.AuthURL, stored.AuthURL), s.HTTPClient)
	statusCode, remoteErr := client.DeleteSession(ctx, stored.AccessToken)
	if err := s.Store.Clear(); err != nil {
		return LogoutResult{}, err
	}

	switch {
	case remoteErr == nil && statusCode == http.StatusNoContent:
		result.LoggedOut = true
		result.RemoteRevoked = true
		return result, nil
	case remoteErr == nil && statusCode == http.StatusUnauthorized:
		result.LoggedOut = true
		return result, nil
	case remoteErr != nil:
		return result, remoteErr
	default:
		result.LoggedOut = true
		return result, nil
	}
}

type resolvedToken struct {
	Token  string
	Source TokenSource
}

func ResolveToken(envToken string, stored Session) resolvedToken {
	if strings.TrimSpace(envToken) != "" {
		return resolvedToken{Token: strings.TrimSpace(envToken), Source: TokenSourceEnv}
	}
	if strings.TrimSpace(stored.AccessToken) != "" {
		return resolvedToken{Token: strings.TrimSpace(stored.AccessToken), Source: TokenSourceStored}
	}
	return resolvedToken{Source: TokenSourceNone}
}

func resolveAuthURL(defaultURL, storedURL string) string {
	if strings.TrimSpace(storedURL) != "" {
		return strings.TrimRight(storedURL, "/")
	}
	return strings.TrimRight(defaultURL, "/")
}

func readAPIError(response *http.Response) error {
	var payload apiError
	if err := json.NewDecoder(response.Body).Decode(&payload); err == nil && payload.Message != "" {
		return errors.New(payload.Message)
	}
	return fmt.Errorf("request failed with status %d", response.StatusCode)
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	return command.Start()
}
