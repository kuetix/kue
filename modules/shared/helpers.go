package shared

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

const DefaultAPIHost = "api.kuetix.com"

type KueConfig struct {
	Host   string                 `json:"host,omitempty"`
	Secure bool                   `json:"secure,omitempty"`
	Login  map[string]interface{} `json:"login,omitempty"`
}

func ResolveAPIHost(hostValues ...string) string {
	for _, host := range hostValues {
		if strings.TrimSpace(host) != "" {
			return NormalizeHost(host)
		}
	}
	if envHost := os.Getenv("KUE_HOST"); strings.TrimSpace(envHost) != "" {
		return NormalizeHost(envHost)
	}
	return NormalizeHost(DefaultAPIHost)
}

func IsHostProvided(hostValues ...string) bool {
	for _, host := range hostValues {
		if strings.TrimSpace(host) != "" {
			return true
		}
	}
	if envHost := os.Getenv("KUE_HOST"); strings.TrimSpace(envHost) != "" {
		return true
	}
	return false
}

func IsHostUseSecure(host string) bool {
	host = strings.TrimSpace(host)
	host = strings.ToLower(host)
	if host == "" {
		return false
	}
	if strings.HasPrefix(host, "https://") {
		return true
	}
	return false
}

func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "https://" + DefaultAPIHost
	}
	if IsHostUseSecure(host) {
		host = strings.TrimPrefix(host, "https://")
		return "https://" + host
	}
	host = strings.TrimPrefix(host, "http://")
	return "http://" + host
}

func DefaultKueConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".kue", "config.json")
}

func ResolveConfigPath(paths ...string) string {
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	if envPath := os.Getenv("KUE_CONFIG_PATH"); strings.TrimSpace(envPath) != "" {
		return envPath
	}
	return DefaultKueConfigPath()
}

func LoadKueConfig(path string) (KueConfig, error) {
	var cfg KueConfig
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func SaveKueConfig(path string, cfg KueConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func GetLoginToken(cfg KueConfig) string {
	if cfg.Login == nil {
		return ""
	}
	for _, key := range []string{"token", "jwt", "access_token"} {
		if value, ok := cfg.Login[key]; ok {
			if token, ok := value.(string); ok {
				return strings.TrimSpace(token)
			}
		}
	}
	if dataValue, ok := cfg.Login["data"]; ok {
		if dataMap, ok := dataValue.(map[string]interface{}); ok {
			for _, key := range []string{"token", "jwt", "access_token"} {
				if value, ok := dataMap[key]; ok {
					if token, ok := value.(string); ok {
						return strings.TrimSpace(token)
					}
				}
			}
		}
	}
	return ""
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func PostLogin(baseHost, username, password string) (map[string]interface{}, error) {
	var err error
	var body []byte
	var requestBody []byte
	var base string
	var url string
	payload := map[string]string{
		"username": username,
		"password": password,
	}
	requestBody, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	urls := make([]string, 2)
	ur := strings.TrimPrefix(baseHost, "http")
	if strings.Contains(ur, "s://") {
		base = strings.TrimPrefix(ur, "s://")
	}
	if strings.Contains(ur, "://") {
		base = strings.TrimPrefix(ur, "://")
	}
	urls[0] = "https://" + base
	urls[1] = "http://" + base
	var resp *http.Response
	defer func() {
		if !resp.Close {
			_ = resp.Body.Close()
		}
	}()
	for _, url = range urls {
		resp = nil
		url = strings.TrimRight(url, "/") + "/auth/login"
		resp, body, err = postLoginDoRequest(url, requestBody)
		if resp != nil && (resp.StatusCode >= 200 || resp.StatusCode < 300) {
			break
		}
	}
	if resp == nil {
		return nil, fmt.Errorf("failed to perform login request: %w", err)
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	var data map[string]interface{}
	if len(body) == 0 {
		return map[string]interface{}{}, nil
	}
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("invalid login response JSON: %w", err)
	}
	return data, nil
}

func postLoginDoRequest(url string, bodyData []byte) (resp *http.Response, body []byte, err error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	return resp, body, nil
}

func PostJSON(baseHost, path string, payload interface{}, headers map[string]string) error {
	var bodyData []byte
	var err error
	if payload != nil {
		bodyData, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	url := strings.TrimRight(baseHost, "/") + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func PerformAuthenticatedRequest(kueConfig KueConfig, method, path string, payload interface{}) (string, int, error) {
	token := GetLoginToken(kueConfig)
	if token == "" {
		return "", http.StatusUnauthorized, fmt.Errorf("no JWT token found. Run 'kue login' first")
	}
	return performRequest(kueConfig, token, method, path, payload)
}

// PerformOptionalAuthRequest behaves like PerformAuthenticatedRequest but
// does not require a login: when no token is stored the request is sent
// anonymously, so it only reaches public resources (e.g. package search).
func PerformOptionalAuthRequest(kueConfig KueConfig, method, path string, payload interface{}) (string, int, error) {
	return performRequest(kueConfig, GetLoginToken(kueConfig), method, path, payload)
}

func performRequest(kueConfig KueConfig, token, method, path string, payload interface{}) (string, int, error) {
	var bodyData []byte
	var err error
	if payload != nil {
		bodyData, err = json.Marshal(payload)
		if err != nil {
			return "", http.StatusInternalServerError, err
		}
	}
	host := FirstNonEmpty(kueConfig.Host, os.Getenv("KUE_HOST"), DefaultAPIHost)
	requestURL := strings.TrimRight(NormalizeHost(host), "/") + path
	req, err := http.NewRequest(method, requestURL, bytes.NewReader(bodyData))
	if err != nil {
		return "", http.StatusInternalServerError, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		code := http.StatusInternalServerError
		if resp != nil {
			code = resp.StatusCode
		}
		return "", code, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", resp.StatusCode, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
	return string(respBody), resp.StatusCode, nil
}

func ReadCredentialsFromStdin() (string, string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", "", err
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", "", err
	}
	input := strings.TrimSpace(string(data))
	if input == "" {
		return "", "", nil
	}
	lines := strings.Split(input, "\n")
	if len(lines) >= 2 {
		return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]), nil
	}
	parts := strings.SplitN(input, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
	}
	return "", "", nil
}

func ReadPasswordFromKeyboard() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return "", nil
	}
	_, _ = fmt.Fprint(os.Stderr, "Password: ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		pass, err := term.ReadPassword(fd)
		_, _ = fmt.Fprintln(os.Stderr)
		if err == nil {
			return string(pass), nil
		}
	}
	return ReadPasswordFromInput(os.Stdin, os.Stderr)
}

func ReadPasswordFromInput(input io.Reader, output io.Writer) (string, error) {
	reader := bufio.NewReader(input)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	_, _ = fmt.Fprintln(output)
	return strings.TrimRight(line, "\r\n"), nil
}
