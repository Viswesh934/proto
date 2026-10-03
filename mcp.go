package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// KnowledgeClient defines the interface for querying the Sanity Knowledge Base.
type KnowledgeClient interface {
	Search(ctx context.Context, query string) ([]KnowledgeResult, error)
}

// JSON-RPC models for MCP protocol
type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type toolCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// SanityMCPClient manages a live connection to a Sanity Context MCP endpoint.
type SanityMCPClient struct {
	endpoint   string
	token      string
	httpClient *http.Client
	sessionID  string
	kbID       string
	tools      []mcpTool
	reqID      int
	mu         sync.Mutex
	initOnce   sync.Once
	initErr    error
}

// NewSanityMCPClient creates a new Sanity Context MCP client.
func NewSanityMCPClient(endpoint, token string) KnowledgeClient {
	if endpoint == "mock" || strings.HasPrefix(endpoint, "mock://") {
		return NewMockKnowledgeClient()
	}

	return &SanityMCPClient{
		endpoint: endpoint,
		token:    token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		reqID: 1,
	}
}

// Search executes a search against the Sanity Knowledge Base via MCP tools.
func (c *SanityMCPClient) Search(ctx context.Context, query string) ([]KnowledgeResult, error) {
	c.initOnce.Do(func() {
		c.initErr = c.initialize(ctx)
	})
	if c.initErr != nil {
		return nil, c.initErr
	}

	hasTool := func(name string) bool {
		for _, t := range c.tools {
			if t.Name == name {
				return true
			}
		}
		return false
	}

	// 1. If initial_context is available and we haven't extracted kbID, call it
	var initialCtxText string
	if hasTool("initial_context") && c.kbID == "" {
		res, err := c.callTool(ctx, "initial_context", map[string]interface{}{})
		if err == nil && len(res.Content) > 0 {
			initialCtxText = res.Content[0].Text
			c.kbID = extractKBID(initialCtxText)
		}
	}

	// 2. Discover relevant entry paths via knowledge_base_search or heuristics
	var targetPaths []string
	searchKeywords := extractKeywords(query)

	if hasTool("knowledge_base_search") && c.kbID != "" {
		searchArgs := map[string]interface{}{
			"knowledgeBase": c.kbID,
			"query":         searchKeywords,
			"return":        "paths",
			"limit":         4,
		}
		res, err := c.callTool(ctx, "knowledge_base_search", searchArgs)
		if err == nil && len(res.Content) > 0 {
			targetPaths = parseSearchPaths(res.Content[0].Text)
		}
	}

	// If no paths returned from search, fallback to heuristic paths
	if len(targetPaths) == 0 {
		targetPaths = fallbackPaths(query)
	}

	// 3. Read entries using knowledge_base_read
	if hasTool("knowledge_base_read") && len(targetPaths) > 0 {
		readArgs := map[string]interface{}{
			"paths": targetPaths,
		}
		if c.kbID != "" {
			readArgs["knowledgeBase"] = c.kbID
		}
		res, err := c.callTool(ctx, "knowledge_base_read", readArgs)
		if err == nil && len(res.Content) > 0 {
			results := parseKnowledgeEntries(res.Content, targetPaths)
			if len(results) > 0 {
				return results, nil
			}
		}
	}

	// 4. Fallback if initial context had content
	if initialCtxText != "" {
		return []KnowledgeResult{
			{
				Title:   "Sanity Context Overview",
				Content: initialCtxText,
				Source:  "Sanity Context Knowledge Base",
			},
		}, nil
	}

	return nil, errors.New("Could not connect to Sanity Context.\n\nCheck SANITY_CONTEXT_MCP_URL and SANITY_CONTEXT_TOKEN.")
}

func (c *SanityMCPClient) initialize(ctx context.Context) error {
	initReq := mcpRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "proto",
				"version": "1.0.0",
			},
		},
	}

	resp, err := c.sendRPC(ctx, initReq)
	if err != nil {
		return errors.New("Could not connect to Sanity Context.\n\nCheck SANITY_CONTEXT_MCP_URL and SANITY_CONTEXT_TOKEN.")
	}

	if resp.Error != nil {
		return errors.New("Could not connect to Sanity Context.\n\nCheck SANITY_CONTEXT_MCP_URL and SANITY_CONTEXT_TOKEN.")
	}

	// Send notifications/initialized
	notifyReq := mcpRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	_, _ = c.sendRPC(ctx, notifyReq)

	// Discover tools
	toolsReq := mcpRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/list",
		Params:  map[string]interface{}{},
	}

	toolsResp, err := c.sendRPC(ctx, toolsReq)
	if err == nil && toolsResp.Result != nil {
		var listRes toolsListResult
		if err := json.Unmarshal(toolsResp.Result, &listRes); err == nil {
			c.tools = listRes.Tools
		}
	}

	return nil
}

func (c *SanityMCPClient) callTool(ctx context.Context, name string, args map[string]interface{}) (*toolCallResult, error) {
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}

	resp, err := c.sendRPC(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	var res toolCallResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *SanityMCPClient) sendRPC(ctx context.Context, rpcReq mcpRequest) (*mcpResponse, error) {
	bodyBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if sess := resp.Header.Get("Mcp-Session-Id"); sess != "" {
		c.sessionID = sess
	}

	// If notification without ID, don't expect a response payload
	if rpcReq.ID == nil {
		return &mcpResponse{JSONRPC: "2.0"}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/event-stream") {
		return parseSSEResponse(resp.Body, rpcReq.ID)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResp mcpResponse
	if err := json.Unmarshal(respBytes, &rpcResp); err != nil {
		return nil, err
	}

	return &rpcResp, nil
}

func (c *SanityMCPClient) nextID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.reqID
	c.reqID++
	return id
}

func parseSSEResponse(r io.Reader, expectedID interface{}) (*mcpResponse, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var rpcResp mcpResponse
			if err := json.Unmarshal([]byte(data), &rpcResp); err == nil {
				return &rpcResp, nil
			}
		}
	}
	return nil, errors.New("no valid response in SSE stream")
}

var kbIDRegex = regexp.MustCompile(`Knowledge base id:\s*` + "`?" + `(kb[A-Za-z0-9]+)` + "`?")

func extractKBID(text string) string {
	m := kbIDRegex.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

var pathRegex = regexp.MustCompile("`([a-zA-Z0-9_/-]+)`")

func parseSearchPaths(text string) []string {
	var paths []string
	seen := make(map[string]bool)

	matches := pathRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			p := m[1]
			if !seen[p] && !strings.HasPrefix(p, "kb") {
				seen[p] = true
				paths = append(paths, p)
			}
		}
	}
	return paths
}

func extractKeywords(query string) string {
	q := strings.ToLower(query)
	var keywords []string

	aipRegex := regexp.MustCompile(`(?i)aip[- ]?(\d+)`)
	if matches := aipRegex.FindAllStringSubmatch(query, -1); len(matches) > 0 {
		for _, m := range matches {
			keywords = append(keywords, "AIP-"+m[1])
		}
	}

	if strings.Contains(q, "put") {
		keywords = append(keywords, "PUT")
	}
	if strings.Contains(q, "patch") {
		keywords = append(keywords, "PATCH")
	}
	if strings.Contains(q, "get") || strings.Contains(q, "retriev") {
		keywords = append(keywords, "Get", "retrieval")
	}
	if strings.Contains(q, "post") || strings.Contains(q, "create") {
		keywords = append(keywords, "Create", "POST")
	}
	if strings.Contains(q, "delete") {
		keywords = append(keywords, "Delete")
	}
	if strings.Contains(q, "idempotenc") {
		keywords = append(keywords, "idempotency")
	}
	if strings.Contains(q, "sub-resource") || strings.Contains(q, "member") || strings.Contains(q, "project") || strings.Contains(q, "parent") || strings.Contains(q, "association") {
		keywords = append(keywords, "sub-resource", "association", "resource_naming")
	}
	if strings.Contains(q, "error") || strings.Contains(q, "problem") || strings.Contains(q, "status") {
		keywords = append(keywords, "error", "status", "Problem Details")
	}

	stopWords := map[string]bool{
		"what": true, "is": true, "are": true, "the": true, "of": true, "to": true,
		"for": true, "in": true, "how": true, "does": true, "a": true, "an": true,
		"and": true, "or": true, "should": true, "i": true, "use": true, "when": true,
		"updating": true, "me": true, "like": true, "im": true, "with": true,
	}

	words := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(q, "?", ""), ",", ""))
	for _, w := range words {
		if len(w) > 2 && !stopWords[w] {
			keywords = append(keywords, w)
		}
	}

	seen := make(map[string]bool)
	var finalKeywords []string
	for _, kw := range keywords {
		kLower := strings.ToLower(kw)
		if !seen[kLower] {
			seen[kLower] = true
			finalKeywords = append(finalKeywords, kw)
		}
	}

	if len(finalKeywords) == 0 {
		return "standard methods HTTP semantics resource-oriented design"
	}

	if len(finalKeywords) > 6 {
		finalKeywords = finalKeywords[:6]
	}

	return strings.Join(finalKeywords, " ")
}

func fallbackPaths(query string) []string {
	q := strings.ToLower(query)
	var paths []string
	if strings.Contains(q, "121") || strings.Contains(q, "124") || strings.Contains(q, "member") || strings.Contains(q, "project") || strings.Contains(q, "parent") {
		paths = append(paths, "resource_oriented_design/philosophy", "resource_naming/names_and_identifiers", "standard_methods")
	} else if strings.Contains(q, "put") || strings.Contains(q, "patch") || strings.Contains(q, "update") || strings.Contains(q, "134") {
		paths = append(paths, "standard_methods", "http_fundamentals", "idempotency_retries")
	} else if strings.Contains(q, "131") || strings.Contains(q, "get") || strings.Contains(q, "retriev") {
		paths = append(paths, "standard_methods", "http_fundamentals", "resource_oriented_design/philosophy")
	} else if strings.Contains(q, "error") || strings.Contains(q, "status") {
		paths = append(paths, "error_handling", "http_fundamentals")
	} else {
		paths = append(paths, "standard_methods", "http_fundamentals", "resource_oriented_design/philosophy")
	}
	return paths
}

func parseKnowledgeEntries(contents []toolContent, requestedPaths []string) []KnowledgeResult {
	var results []KnowledgeResult

	for _, item := range contents {
		if item.Type != "text" || strings.TrimSpace(item.Text) == "" {
			continue
		}

		rawEntries := splitMarkdownEntries(item.Text)
		for i, entryText := range rawEntries {
			entryText = strings.TrimSpace(entryText)
			if entryText == "" {
				continue
			}

			title := extractTitleFromContent(entryText)
			if title == "" && i < len(requestedPaths) {
				title = formatPathAsTitle(requestedPaths[i])
			}
			if title == "" {
				title = fmt.Sprintf("Sanity Entry %d", len(results)+1)
			}

			source := extractSourceFromContent(entryText)
			if source == "" {
				source = "Sanity Context Knowledge Base"
			}

			results = append(results, KnowledgeResult{
				Title:   title,
				Content: entryText,
				Source:  source,
			})
		}
	}

	return results
}

func splitMarkdownEntries(text string) []string {
	lines := strings.Split(text, "\n")
	var entries []string
	var current strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") && current.Len() > 0 {
			entries = append(entries, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		entries = append(entries, current.String())
	}

	return entries
}

func extractTitleFromContent(text string) string {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "# ") {
			return strings.TrimPrefix(l, "# ")
		}
	}
	return ""
}

func formatPathAsTitle(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Title(name)
}

func extractSourceFromContent(text string) string {
	if idx := strings.LastIndex(text, "## Sources"); idx != -1 {
		sourcesBlock := text[idx:]
		lines := strings.Split(sourcesBlock, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "1. ") || strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "• ") {
				return strings.TrimLeft(l, "1234567890. -•")
			}
		}
	}
	return ""
}

// MockKnowledgeClient provides canned high-fidelity knowledge entries for local demos & testing.
type MockKnowledgeClient struct{}

func NewMockKnowledgeClient() KnowledgeClient {
	return &MockKnowledgeClient{}
}

func (m *MockKnowledgeClient) Search(ctx context.Context, query string) ([]KnowledgeResult, error) {
	q := strings.ToLower(query)

	if strings.Contains(q, "project") || strings.Contains(q, "member") || strings.Contains(q, "121") || strings.Contains(q, "124") || strings.Contains(q, "sub-resource") {
		return []KnowledgeResult{
			{
				Title: "AIP-121: Resource-oriented design",
				Content: `AIP-121 defines the foundation of resource-oriented API design:
- An API is modeled as a resource hierarchy where each node is either a simple resource or a collection of resources of the same type.
- Resources must be named with nouns rather than RPC verbs (e.g. /v1/projects rather than /createProject).
- Standard methods: Create, Get, List, Update, Delete.
- Sub-resources represent entities contained within a parent (e.g. /v1/projects/{project}/members).
- Relationships must form a directed acyclic graph.`,
				Source: "AIP-121 (google.aip.dev/121)",
			},
			{
				Title: "AIP-124: Resource association and sub-resources",
				Content: `AIP-124 defines patterns for modeling associations between resources:
- Canonical parent rule: A resource must have at most one canonical parent.
- When an entity's existence is completely scoped to a parent (e.g. project members), model it as a nested sub-collection: /projects/{project}/members.
- List RPCs for sub-resources must treat the parent as required.
- Deleting the parent resource must cascade to delete its nested sub-resources.`,
				Source: "AIP-124 (google.aip.dev/124)",
			},
			{
				Title: "AIP-133: Standard methods: Create",
				Content: `AIP-133 defines standard Create methods:
- HTTP method: POST.
- The URI must be the collection URI: POST /v1/{parent=projects/*}/members or POST /v1/projects.
- The request body contains the resource to create.
- The response returns the created resource with HTTP 200 OK or 201 Created.`,
				Source: "AIP-133 (google.aip.dev/133)",
			},
		}, nil
	}

	if strings.Contains(q, "patch") || strings.Contains(q, "put") || strings.Contains(q, "update") {
		return []KnowledgeResult{
			{
				Title: "AIP-134: Standard methods: Update",
				Content: `AIP-134 defines the standard Update method for resource-oriented APIs.
- Update methods should support partial updates using HTTP PATCH.
- Full resource replacement may use HTTP PUT, but PUT on sub-properties (e.g. PUT /users/{id}/email) is an anti-pattern; resource fields should be updated via PATCH /users/{id} with an update mask or partial document.
- Update methods must be idempotent.
- The URI should identify the resource being updated, e.g., PATCH /v1/users/{user}.`,
				Source: "AIP-134 (google.aip.dev/134)",
			},
			{
				Title: "RFC 9110: HTTP Semantics — Section 9.3.4 (PUT) & 9.3.8 (PATCH)",
				Content: `PUT requests that the target resource state be replaced with the state defined by the representation in the request payload.
PATCH applies partial modifications to a resource. Updating individual fields via PUT directly on a sub-path creates unnecessary sub-resources and violates resource containment.`,
				Source: "RFC 9110 (HTTP Semantics)",
			},
			{
				Title: "API Resource Design: Resource Identity and Sub-properties",
				Content: `Sub-properties of a resource should not be exposed as separate top-level endpoints unless they constitute an independent collection. Update properties on the primary resource representation rather than creating endpoints like /users/{id}/email.`,
				Source: "API Resource Design Principles",
			},
		}, nil
	}

	if strings.Contains(q, "error") || strings.Contains(q, "status") {
		return []KnowledgeResult{
			{
				Title: "RFC 7807 & RFC 9457: Problem Details for HTTP APIs",
				Content: `Problem details define a standard JSON structure for carrying machine-readable details of errors in HTTP responses:
- "type": A URI reference that identifies the problem type.
- "title": A short, human-readable summary of the problem type.
- "status": The HTTP status code generated by the origin server.
- "detail": A human-readable explanation specific to this occurrence.
- "instance": A URI reference identifying the specific occurrence of the problem.`,
				Source: "RFC 9457 (Problem Details)",
			},
			{
				Title: "AIP-193: Errors and Status Codes",
				Content: `Standard HTTP status codes must be used to indicate API outcome:
- 200 OK for successful retrieval.
- 404 Not Found when a requested resource does not exist.
- 400 Bad Request for malformed request payloads or invalid parameters.
Errors should return standard error payloads rather than empty bodies or 200 OK with error flags.`,
				Source: "AIP-193 (google.aip.dev/193)",
			},
		}, nil
	}

	// Default: Retrieval / GET / POST /getUser
	return []KnowledgeResult{
		{
			Title: "AIP-131: Standard methods: Get",
			Content: `AIP-131 defines the standard Get method for retrieving an individual resource.
- An endpoint that retrieves a single resource must use HTTP GET.
- The URI must identify the specific resource being retrieved, e.g. GET /v1/{name=users/*}.
- The request must not have a request body.
- Get methods must be safe (read-only) and idempotent.
- Using HTTP POST for retrieval is an anti-pattern and violates standard HTTP semantics and resource-oriented API design.`,
			Source: "AIP-131 (google.aip.dev/131)",
		},
		{
			Title: "RFC 9110: HTTP Semantics — Section 9.3.1 (GET) & 9.3.3 (POST)",
			Content: `GET is defined as a safe method; safe methods are HTTP methods that are essentially read-only.
POST is intended for non-safe, non-idempotent operations such as creating resources or submitting form data.
Using POST for information retrieval bypasses HTTP caching, proxies, and standard retry semantics.`,
			Source: "RFC 9110 (HTTP Semantics)",
		},
		{
			Title: "Resource-Oriented API Design: Naming and Hierarchy",
			Content: `Resource-oriented APIs expose collections and individual resources using nouns rather than RPC verbs.
- Noun-based: /v1/users/{user_id}
- Anti-pattern: /getUser, /fetchUser, /api/v1/user/get
Retrieval operations must map to HTTP GET on the resource URI rather than custom action endpoints.`,
			Source: "API Resource Design Principles",
		},
	}, nil
}
