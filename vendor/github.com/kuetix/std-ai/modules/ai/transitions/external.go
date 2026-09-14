package transitions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

type externalTransitions struct {
	workflow.BaseServiceTransition
}

func NewExternalTransitions() interfaces.ServiceTransitions {
	return &externalTransitions{}
}

func toStringMap(values map[string]interface{}) map[string]string {
	result := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		result[key] = fmt.Sprintf("%v", value)
	}
	return result
}

func toStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, fmt.Sprintf("%v", item))
		}
		return result
	default:
		if value == nil {
			return nil
		}
		return []string{fmt.Sprintf("%v", value)}
	}
}

func toMap(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return copyMap(v)
	default:
		return map[string]interface{}{}
	}
}

func mapToURLValues(values map[string]interface{}) url.Values {
	result := url.Values{}
	for key, value := range values {
		switch v := value.(type) {
		case []interface{}:
			for _, item := range v {
				result.Add(key, fmt.Sprintf("%v", item))
			}
		case []string:
			for _, item := range v {
				result.Add(key, item)
			}
		default:
			result.Set(key, fmt.Sprintf("%v", value))
		}
	}
	return result
}

func parseResponseBody(data []byte, contentType string) interface{} {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(contentType, "application/json") {
		var payload interface{}
		if err := json.Unmarshal(data, &payload); err == nil {
			return payload
		}
	}

	var payload interface{}
	if err := json.Unmarshal(data, &payload); err == nil {
		return payload
	}
	return string(data)
}

func parseTimeoutSeconds(value interface{}, defaultValue int) int {
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && i > 0 {
			return i
		}
	}
	return defaultValue
}

func aggregateMCPContent(content []mcp.Content) string {
	textParts := make([]string, 0, len(content))
	for _, item := range content {
		switch value := item.(type) {
		case mcp.TextContent:
			if strings.TrimSpace(value.Text) != "" {
				textParts = append(textParts, value.Text)
			}
		case *mcp.TextContent:
			if value != nil && strings.TrimSpace(value.Text) != "" {
				textParts = append(textParts, value.Text)
			}
		default:
			encoded, err := json.Marshal(value)
			if err == nil {
				textParts = append(textParts, string(encoded))
			}
		}
	}
	return strings.Join(textParts, "\n")
}

func initializeMCPClient(ctx context.Context, client *mcpclient.Client) error {
	if err := client.Start(ctx); err != nil {
		return err
	}
	_, err := client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "std-ai",
				Version: "0.1.0",
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	})
	return err
}

// HTTP calls an external HTTP endpoint.
func (t *externalTransitions) HTTP(method, requestURL string, headers, query map[string]interface{}, body interface{}, timeoutSeconds int) (r domain.FlowStepResult) {
	requestURL = strings.TrimSpace(requestURL)
	if requestURL == "" {
		r.Error = fmt.Errorf("requestURL is required")
		r.StatusCode = http.StatusBadRequest
		return
	}

	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		r.Error = fmt.Errorf("invalid requestURL: %w", err)
		r.StatusCode = http.StatusBadRequest
		return
	}
	if len(query) > 0 {
		values := parsedURL.Query()
		for key, vals := range mapToURLValues(query) {
			for _, value := range vals {
				values.Add(key, value)
			}
		}
		parsedURL.RawQuery = values.Encode()
	}

	var requestBody io.Reader
	if body != nil && method != http.MethodGet && method != http.MethodHead {
		encoded, err := json.Marshal(body)
		if err != nil {
			r.Error = fmt.Errorf("failed to marshal request body: %w", err)
			r.StatusCode = http.StatusBadRequest
			return
		}
		requestBody = bytes.NewReader(encoded)
	}

	request, err := http.NewRequest(method, parsedURL.String(), requestBody)
	if err != nil {
		r.Error = fmt.Errorf("failed to create request: %w", err)
		r.StatusCode = http.StatusInternalServerError
		return
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	applyAdditionalHeaders(request, headers)

	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	response, err := client.Do(request)
	if err != nil {
		r.Error = fmt.Errorf("http request failed: %w", err)
		r.StatusCode = http.StatusBadGateway
		return
	}
	defer func() { _ = response.Body.Close() }()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		r.Error = fmt.Errorf("failed to read response body: %w", err)
		r.StatusCode = http.StatusBadGateway
		return
	}

	result := map[string]interface{}{
		"method":     method,
		"url":        parsedURL.String(),
		"statusCode": response.StatusCode,
		"headers":    response.Header,
		"body":       parseResponseBody(responseBody, response.Header.Get("Content-Type")),
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		r.Error = fmt.Errorf("http request failed with status %d", response.StatusCode)
		r.StatusCode = response.StatusCode
		r.Response = result
		return
	}

	r.Success = true
	r.StatusCode = response.StatusCode
	r.Response = result
	return
}

// MCP calls a remote MCP tool over streamable HTTP, SSE, or stdio.
func (t *externalTransitions) MCP(transport, target, tool string, headers, input map[string]interface{}, timeoutSeconds int, command string, args, env []interface{}) (r domain.FlowStepResult) {
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport == "" {
		transport = "http"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	tool = strings.TrimSpace(tool)
	if tool == "" {
		r.Error = fmt.Errorf("tool is required")
		r.StatusCode = http.StatusBadRequest
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var (
		client *mcpclient.Client
		err    error
	)

	switch transport {
	case "http", "streamable-http":
		target = strings.TrimSpace(target)
		if target == "" {
			r.Error = fmt.Errorf("target is required for MCP transport %q", transport)
			r.StatusCode = http.StatusBadRequest
			return
		}
		client, err = mcpclient.NewStreamableHttpClient(
			target,
			mcptransport.WithHTTPHeaders(toStringMap(headers)),
			mcptransport.WithHTTPBasicClient(&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}),
		)
	case "sse":
		target = strings.TrimSpace(target)
		if target == "" {
			r.Error = fmt.Errorf("target is required for MCP transport %q", transport)
			r.StatusCode = http.StatusBadRequest
			return
		}
		client, err = mcpclient.NewSSEMCPClient(
			target,
			mcpclient.WithHeaders(toStringMap(headers)),
			mcpclient.WithHTTPClient(&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}),
		)
	case "stdio":
		command = firstNonEmpty(command, target)
		if command == "" {
			r.Error = fmt.Errorf("command is required for stdio MCP transport")
			r.StatusCode = http.StatusBadRequest
			return
		}
		client, err = mcpclient.NewStdioMCPClient(command, toStringSlice(env), toStringSlice(args)...)
	default:
		r.Error = fmt.Errorf("unsupported MCP transport %q", transport)
		r.StatusCode = http.StatusBadRequest
		return
	}
	if err != nil {
		r.Error = fmt.Errorf("failed to create MCP client: %w", err)
		r.StatusCode = http.StatusBadGateway
		return
	}
	defer func() { _ = client.Close() }()

	if err := initializeMCPClient(ctx, client); err != nil {
		r.Error = fmt.Errorf("failed to initialize MCP client: %w", err)
		r.StatusCode = http.StatusBadGateway
		return
	}

	callResult, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      tool,
			Arguments: input,
		},
	})
	if err != nil {
		r.Error = fmt.Errorf("failed to call MCP tool %q: %w", tool, err)
		r.StatusCode = http.StatusBadGateway
		return
	}

	result := map[string]interface{}{
		"transport":         transport,
		"target":            target,
		"tool":              tool,
		"text":              aggregateMCPContent(callResult.Content),
		"content":           callResult.Content,
		"structuredContent": callResult.StructuredContent,
		"isError":           callResult.IsError,
	}
	if callResult.IsError {
		r.Error = fmt.Errorf("mcp tool %q returned an error", tool)
		r.StatusCode = http.StatusBadGateway
		r.Response = result
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = result
	return
}

// Execute routes to an external adapter implementation. Supported adapters are http and mcp.
func (t *externalTransitions) Execute(adapter, target string, options, input map[string]interface{}) (r domain.FlowStepResult) {
	adapter = strings.ToLower(strings.TrimSpace(adapter))
	switch adapter {
	case agentToolHTTP:
		method := firstNonEmpty(valueAsString(options["method"]), http.MethodPost)
		headers := mergeMaps(toMap(options["headers"]), nil)
		query := toMap(options["query"])
		body := options["body"]
		if bodyMap, ok := body.(map[string]interface{}); ok {
			body = mergeMaps(bodyMap, input)
		} else if body == nil {
			if strings.EqualFold(method, http.MethodGet) || strings.EqualFold(method, http.MethodDelete) {
				query = mergeMaps(query, input)
			} else {
				body = input
			}
		}
		return t.HTTP(method, target, headers, query, body, parseTimeoutSeconds(options["timeoutSeconds"], 30))
	case agentToolMCP:
		return t.MCP(
			firstNonEmpty(valueAsString(options["transport"]), "http"),
			target,
			valueAsString(options["tool"]),
			toMap(options["headers"]),
			input,
			parseTimeoutSeconds(options["timeoutSeconds"], 30),
			valueAsString(options["command"]),
			anySlice(toStringSlice(options["args"])),
			anySlice(toStringSlice(options["env"])),
		)
	default:
		r.Error = fmt.Errorf("unsupported external adapter %q", adapter)
		r.StatusCode = http.StatusBadRequest
		return
	}
}

func anySlice(values []string) []interface{} {
	if len(values) == 0 {
		return nil
	}
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
