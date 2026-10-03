package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// LLMClient defines the interface for interacting with the language model.
type LLMClient interface {
	PlanResearch(ctx context.Context, question string) (*ResearchPlan, error)
	AnswerQuestion(ctx context.Context, question string, researchContext string) (*Answer, error)

	UnderstandAndQuery(ctx context.Context, api *APIRequest) (string, string, error)
	Review(ctx context.Context, api *APIRequest, knowledge []KnowledgeResult) (*ReviewResult, error)
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// OpenRouterClient implements LLMClient using the OpenRouter chat completions API.
type OpenRouterClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewOpenRouterClient creates a new OpenRouter client.
func NewOpenRouterClient(apiKey, model string) LLMClient {
	if apiKey == "mock" || apiKey == "" && os.Getenv("PROTO_MOCK") == "true" {
		return NewMockLLMClient()
	}

	if model == "" {
		model = "google/gemini-2.5-flash"
	}

	baseURL := os.Getenv("OPENROUTER_BASE_URL")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	return &OpenRouterClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// PlanResearch generates a structured research plan targeting the Sanity Knowledge Base.
func (c *OpenRouterClient) PlanResearch(ctx context.Context, question string) (*ResearchPlan, error) {
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMessage{
			{Role: "system", Content: SystemPromptResearchPlanner},
			{Role: "user", Content: BuildResearchPlannerPrompt(question)},
		},
		Temperature: 0.1,
		MaxTokens:   800,
	}

	respStr, err := c.sendChatCompletion(ctx, reqBody)
	if err != nil {
		return fallbackPlanResearch(question), nil
	}

	cleaned := cleanJSONOutput(respStr)
	var plan ResearchPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil || len(plan.Questions) == 0 {
		return fallbackPlanResearch(question), nil
	}

	if plan.QuestionType == "" {
		plan.QuestionType = "API design inquiry"
	}

	return &plan, nil
}

// AnswerQuestion synthesizes the final source-backed answer using retrieved Sanity knowledge.
func (c *OpenRouterClient) AnswerQuestion(ctx context.Context, question string, researchContext string) (*Answer, error) {
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMessage{
			{Role: "system", Content: SystemPromptResearchAnswer},
			{Role: "user", Content: BuildResearchAnswerPrompt(question, researchContext)},
		},
		Temperature: 0.2,
		MaxTokens:   3500,
	}

	respStr, err := c.sendChatCompletion(ctx, reqBody)
	if err != nil {
		return nil, errors.New("Unable to generate the final answer.")
	}

	cleaned := cleanJSONOutput(respStr)
	var ans Answer
	if err := json.Unmarshal([]byte(cleaned), &ans); err != nil {
		if repaired := repairOrExtractAnswer(cleaned); repaired != nil && repaired.Answer != "" {
			return repaired, nil
		}
		return nil, errors.New("Unable to generate the final answer.")
	}

	return &ans, nil
}

// UnderstandAndQuery asks the LLM to understand the API request and form a Sanity research query.
func (c *OpenRouterClient) UnderstandAndQuery(ctx context.Context, api *APIRequest) (string, string, error) {
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMessage{
			{Role: "system", Content: SystemPromptUnderstandAndQuery},
			{Role: "user", Content: BuildUnderstandAndQueryUserPrompt(api)},
		},
		Temperature: 0.1,
		MaxTokens:   800,
	}

	respStr, err := c.sendChatCompletion(ctx, reqBody)
	if err != nil {
		understanding, query := fallbackUnderstandAndQuery(api)
		return understanding, query, nil
	}

	cleaned := cleanJSONOutput(respStr)
	var parsed struct {
		Understanding string `json:"understanding"`
		Query         string `json:"query"`
	}

	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil || parsed.Query == "" {
		understanding, query := fallbackUnderstandAndQuery(api)
		return understanding, query, nil
	}

	if parsed.Understanding == "" {
		parsed.Understanding = "API inspection in progress"
	}

	return parsed.Understanding, parsed.Query, nil
}

// Review submits the API request and Sanity knowledge to the LLM for design review.
func (c *OpenRouterClient) Review(ctx context.Context, api *APIRequest, knowledge []KnowledgeResult) (*ReviewResult, error) {
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMessage{
			{Role: "system", Content: SystemPromptReview},
			{Role: "user", Content: BuildReviewUserPrompt(api, knowledge)},
		},
		Temperature: 0.1,
		MaxTokens:   1500,
	}

	respStr, err := c.sendChatCompletion(ctx, reqBody)
	if err != nil {
		return nil, errors.New("LLM request failed.")
	}

	cleaned := cleanJSONOutput(respStr)
	var result ReviewResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, errors.New("LLM request failed.")
	}

	return &result, nil
}

func (c *OpenRouterClient) sendChatCompletion(ctx context.Context, reqBody openRouterRequest) (string, error) {
	if c.apiKey == "" {
		return "", errors.New("OPENROUTER_API_KEY is not configured.")
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/proto/proto")
	httpReq.Header.Set("X-Title", "Proto API Reviewer")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status code %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed openRouterResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", err
	}

	if parsed.Error != nil {
		return "", fmt.Errorf("openrouter error: %s", parsed.Error.Message)
	}

	if len(parsed.Choices) == 0 {
		return "", errors.New("no choices returned")
	}

	return parsed.Choices[0].Message.Content, nil
}

func cleanJSONOutput(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func repairOrExtractAnswer(cleaned string) *Answer {
	closingAttempts := []string{
		`"]}`,
		`"]}]}`,
		`"}]}`,
		`"}`,
		`"]`,
		`}`,
	}
	for _, closing := range closingAttempts {
		var ans Answer
		if err := json.Unmarshal([]byte(cleaned+closing), &ans); err == nil && ans.Answer != "" {
			return &ans
		}
	}

	ans := &Answer{Summary: "API guidance recommendation"}
	summaryRe := regexp.MustCompile(`"summary"\s*:\s*"([^"]+)"`)
	if m := summaryRe.FindStringSubmatch(cleaned); len(m) > 1 {
		ans.Summary = m[1]
	}

	answerRe := regexp.MustCompile(`"answer"\s*:\s*"((?:[^"\\]|\\.)*)`)
	if m := answerRe.FindStringSubmatch(cleaned); len(m) > 1 {
		var unescaped string
		if json.Unmarshal([]byte(`"`+m[1]+`"`), &unescaped) == nil {
			ans.Answer = unescaped
			return ans
		}
		ans.Answer = strings.ReplaceAll(m[1], `\n`, "\n")
		return ans
	}

	return nil
}

func fallbackPlanResearch(question string) *ResearchPlan {
	q := strings.ToLower(question)

	if strings.Contains(q, "put") && strings.Contains(q, "patch") {
		return &ResearchPlan{
			QuestionType: "API design comparison",
			Questions: []string{
				"What are the semantics of PUT?",
				"What are the semantics of PATCH?",
				"How are partial updates represented in resource-oriented APIs?",
				"What are the idempotency implications of update methods?",
			},
		}
	}

	if strings.Contains(q, "131") || (strings.Contains(q, "get") && strings.Contains(q, "explain")) {
		return &ResearchPlan{
			QuestionType: "API concept explanation",
			Questions: []string{
				"What is AIP-131 standard Get method specification?",
				"What are the URI patterns and HTTP requirements for Get methods?",
				"Why are request bodies prohibited in standard retrieval operations?",
			},
		}
	}

	if strings.Contains(q, "getuser") || (strings.Contains(q, "post") && strings.Contains(q, "get")) {
		return &ResearchPlan{
			QuestionType: "API design review",
			Questions: []string{
				"What are the HTTP semantics of GET vs POST for retrieval?",
				"What does AIP-131 mandate for standard Get methods?",
				"How should resource-oriented URIs be structured for entity retrieval?",
			},
		}
	}

	if strings.Contains(q, "project") || strings.Contains(q, "member") || strings.Contains(q, "design") {
		return &ResearchPlan{
			QuestionType: "Resource-oriented API design",
			Questions: []string{
				"How are parent-child collections modeled in resource-oriented design (AIP-121)?",
				"What are standard CRUD method patterns for resource creation and deletion?",
				"How are sub-resources and member associations structured (AIP-124)?",
			},
		}
	}

	return &ResearchPlan{
		QuestionType: "API design inquiry",
		Questions: []string{
			fmt.Sprintf("What API standards apply to: %s", question),
			"What do HTTP semantics and resource-oriented guidelines recommend?",
		},
	}
}

func fallbackUnderstandAndQuery(api *APIRequest) (string, string) {
	pathLower := strings.ToLower(api.Path)
	method := strings.ToUpper(api.Method)

	if method == "POST" && (strings.Contains(pathLower, "get") || strings.Contains(pathLower, "fetch") || strings.Contains(pathLower, "find") || strings.Contains(pathLower, "read")) {
		return "Retrieval operation detected",
			fmt.Sprintf("Review an API endpoint that uses POST for retrieving an individual resource at %s. Find relevant guidance about HTTP method semantics, standard Get methods, resource-oriented API design, and CRUD operations.", api.Path)
	}

	if method == "PUT" && strings.Contains(pathLower, "/") {
		return "Resource update operation detected",
			fmt.Sprintf("Find guidance about PUT semantics versus PATCH semantics, partial updates, idempotency, resource-oriented API design, and sub-resource properties for %s.", api.Path)
	}

	if method == "PATCH" {
		return "Partial resource update detected",
			fmt.Sprintf("Find guidance about PATCH semantics, partial updates, idempotency, resource-oriented API design, and HTTP method behavior for %s.", api.Path)
	}

	if method == "GET" {
		return "Resource retrieval detected",
			fmt.Sprintf("Find guidance about standard GET method conventions, URI design, error handling status codes, and Problem Details for %s.", api.Path)
	}

	return "API operation detected",
		fmt.Sprintf("Find relevant API design standards, HTTP semantics, and resource-oriented guidelines for %s %s.", api.Method, api.Path)
}

// MockLLMClient provides deterministic, high-quality reviews for testing and offline scenarios.
type MockLLMClient struct{}

func NewMockLLMClient() LLMClient {
	return &MockLLMClient{}
}

func (m *MockLLMClient) PlanResearch(ctx context.Context, question string) (*ResearchPlan, error) {
	return fallbackPlanResearch(question), nil
}

func (m *MockLLMClient) AnswerQuestion(ctx context.Context, question string, researchContext string) (*Answer, error) {
	q := strings.ToLower(question)

	if strings.Contains(q, "put") && strings.Contains(q, "patch") {
		return &Answer{
			Summary: "Use PATCH for updating specific fields like an email address. PUT is reserved for full resource replacement.",
			Answer: `When updating an individual field such as a user's email, **PATCH** is the correct HTTP method according to standard REST principles and API Improvement Proposals (AIP-134).

### Key Differences Between PUT and PATCH

1. **HTTP Semantics (RFC 9110 § 9.3.4 & 9.3.8)**:
   - **PUT**: Semantically replaces the *entire* target resource state. If you send only ` + "`" + `{"email": "x@example.com"}` + "`" + ` in a PUT request, any omitted fields (name, preferences, etc.) should technically be cleared or reset to defaults.
   - **PATCH**: Specifically defined for *partial modifications*. The server applies only the changes described in the request payload.

2. **AIP-134 (Standard Methods: Update)**:
   - Resource-oriented APIs should implement Update using **HTTP PATCH**.
   - Partial updates should target the parent resource URI (` + "`" + `PATCH /v1/users/{user}` + "`" + `) rather than inventing sub-property endpoints like ` + "`" + `PUT /users/{id}/email` + "`" + `.
   - Use field masks or partial representations to indicate which fields are being updated.

3. **Idempotency & Safety**:
   - Both PUT and PATCH operations in standard CRUD must be idempotent.
   - Using PATCH prevents accidental data loss from race conditions where two clients read-modify-write different fields simultaneously.

### Recommended Design

` + "```http\nPATCH /v1/users/{user}\nContent-Type: application/json\n\n{\n  \"email\": \"new-email@example.com\"\n}\n```",
			Guidance: []string{
				"AIP-134 — Standard methods: Update",
				"RFC 9110 — HTTP Semantics (PUT vs PATCH)",
				"API Resource Design Principles",
			},
			Sources: []Source{
				{Title: "AIP-134: Standard methods: Update", URL: "https://google.aip.dev/134"},
				{Title: "RFC 9110: HTTP Semantics (Section 9.3.4 & 9.3.8)"},
			},
		}, nil
	}

	if strings.Contains(q, "131") {
		return &Answer{
			Summary: "AIP-131 defines the standard Get method for retrieving a single resource by its unique identifier.",
			Answer: `**AIP-131** is Google's API Improvement Proposal for standard **Get** methods in resource-oriented APIs.

### The Plain-English Breakdown for Backend Developers

Think of AIP-131 as the gold standard rulebook for fetching a single entity (like a user, order, or document):

1. **HTTP Method Must Be GET**:
   - Retrieval operations must be **safe** (read-only) and **idempotent**.
   - Calling it 10 times should leave the server state unchanged.

2. **No Request Body**:
   - Standard HTTP GET requests must never include a body payload. Any filtering or identifiers belong in the URL path.

3. **URL Identifies the Resource**:
   - Structure: ` + "`" + `GET /v1/{name=users/*}` + "`" + `
   - Good: ` + "`" + `GET /v1/users/123` + "`" + `
   - Bad: ` + "`" + `POST /getUser` + "`" + ` or ` + "`" + `GET /api/fetchUser?id=123` + "`" + `

4. **Response Is the Resource Itself**:
   - The response payload is the full resource representation, with standard HTTP 200 OK or 404 NOT_FOUND.`,
			Guidance: []string{
				"AIP-131 — Standard methods: Get",
				"AIP-121 — Resource-oriented design",
				"RFC 9110 — HTTP Semantics: Safe Methods",
			},
			Sources: []Source{
				{Title: "AIP-131: Standard methods: Get", URL: "https://google.aip.dev/131"},
				{Title: "AIP-121: Resource-oriented design", URL: "https://google.aip.dev/121"},
			},
		}, nil
	}

	if strings.Contains(q, "project") && strings.Contains(q, "member") {
		return &Answer{
			Summary: "Design a resource-oriented API with top-level projects collection and nested members sub-collection.",
			Answer: `Here is a complete, resource-oriented API design adhering to **AIP-121** and **AIP-124**:

### Recommended Endpoint Structure

` + "```http\n# 1. Projects Collection\nPOST   /v1/projects                       # Create a project\nGET    /v1/projects                       # List projects\nGET    /v1/projects/{project}             # Get project details\nPATCH  /v1/projects/{project}             # Update project\nDELETE /v1/projects/{project}             # Delete project\n\n# 2. Project Members Sub-collection\nPOST   /v1/projects/{project}/members     # Add a member\nGET    /v1/projects/{project}/members     # List members\nGET    /v1/projects/{project}/members/{member} # Get member status\nDELETE /v1/projects/{project}/members/{member} # Remove member\n```" + `

### Rationale Based on Retrieved Guidance

1. **Noun-Based Resource Hierarchy (AIP-121)**:
   - APIs represent entities as nouns. Projects is a top-level collection, and Members is a sub-collection contained within a specific project.
2. **Sub-resource Associations (AIP-124)**:
   - Because member permissions are scoped directly to a specific project parent, nesting members under ` + "`" + `/projects/{project}/members` + "`" + ` enforces parent containment and acyclic associations.
3. **Standard CRUD Verbs**:
   - All standard operations map cleanly to standard HTTP methods (POST create, GET retrieve, PATCH update, DELETE remove).`,
			Guidance: []string{
				"AIP-121 — Resource-oriented design",
				"AIP-124 — Resource association and sub-resources",
				"AIP-133 — Standard methods: Create",
			},
			Sources: []Source{
				{Title: "AIP-121: Resource-oriented design", URL: "https://google.aip.dev/121"},
				{Title: "AIP-124: Resource association", URL: "https://google.aip.dev/124"},
			},
		}, nil
	}

	// Default fallback
	return &Answer{
		Summary: "API design analysis grounded in Sanity API standards.",
		Answer:  fmt.Sprintf("Based on the retrieved Sanity Knowledge Base guidance, here is the architectural recommendation for: %s\n\nAlways follow HTTP semantics (RFC 9110) and Google AIP standards.", question),
		Guidance: []string{
			"RFC 9110 — HTTP Semantics",
			"AIP-121 — Resource-oriented design",
		},
		Sources: []Source{
			{Title: "RESTICE API Standards"},
		},
	}, nil
}

func (m *MockLLMClient) UnderstandAndQuery(ctx context.Context, api *APIRequest) (string, string, error) {
	understanding, query := fallbackUnderstandAndQuery(api)
	return understanding, query, nil
}

func (m *MockLLMClient) Review(ctx context.Context, api *APIRequest, knowledge []KnowledgeResult) (*ReviewResult, error) {
	pathLower := strings.ToLower(api.Path)
	method := strings.ToUpper(api.Method)

	if method == "POST" && (strings.Contains(pathLower, "get") || strings.Contains(pathLower, "find") || strings.Contains(pathLower, "fetch")) {
		return &ReviewResult{
			Summary: "The endpoint appears to retrieve a resource using HTTP POST.",
			Findings: []Finding{
				{
					Severity:    "warning",
					Issue:       "POST is being used for a retrieval operation.",
					Explanation: "This endpoint appears to retrieve a resource.",
					Guidance: []string{
						"AIP-131 — Standard methods: Get",
					},
					Suggestion: "GET /v1/users/{user}",
				},
			},
			Sources: []string{
				"AIP-131",
				"HTTP semantics",
				"API resource design",
			},
		}, nil
	}

	return &ReviewResult{
		Summary: "Standard API inspection.",
		Findings: []Finding{
			{
				Severity:    "info",
				Issue:       "Conforms to standards.",
				Explanation: "API conforms to guidelines.",
				Guidance:    []string{"AIP-131"},
				Suggestion:  "No change needed",
			},
		},
		Sources: []string{"AIP-131"},
	}, nil
}
