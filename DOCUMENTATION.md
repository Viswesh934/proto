# Proto: Phase 1 & Phase 2 Technical Documentation

## Table of Contents
1. [Overview & Challenge Theme](#1-overview--challenge-theme)
2. [Architecture & System Flow](#2-architecture--system-flow)
3. [Phase 1: Terminal API Design Reviewer](#3-phase-1-terminal-api-design-reviewer)
   - [Phase 1 Scope & Objectives](#phase-1-scope--objectives)
   - [Curl Parser (`parser.go`)](#curl-parser-parsergo)
   - [Sanity Context MCP Client (`mcp.go`)](#sanity-context-mcp-client-mcpgo)
   - [API Review Engine & Output Format](#api-review-engine--output-format)
   - [Phase 1 Verified Scenarios](#phase-1-verified-scenarios)
4. [Phase 2: Natural Language API Design Research Agent](#4-phase-2-natural-language-api-design-research-agent)
   - [Phase 2 Scope & Objectives](#phase-2-scope--objectives)
   - [The Five-Stage Research Workflow](#the-five-stage-research-workflow)
   - [Research Planning & Intent Classification](#stage-1--2-intent-classification--research-planning)
   - [Dynamic Multi-Query MCP Retrieval](#stage-3-dynamic-multi-query-mcp-retrieval)
   - [Research Synthesis & Normalized Context](#stage-4--5-normalized-context--grounded-synthesis)
   - [Phase 2 Verified Scenarios](#phase-2-verified-scenarios)
5. [Sanity Context MCP Integration Deep-Dive](#5-sanity-context-mcp-integration-deep-dive)
   - [Protocol & Transport Layer](#protocol--transport-layer)
   - [Tool Orchestration Sequence](#tool-orchestration-sequence)
   - [Why Structured Knowledge Bases Matter](#why-structured-knowledge-bases-matter)
6. [OpenRouter LLM Integration Deep-Dive](#6-openrouter-llm-integration-deep-dive)
   - [Prompt Engineering & Anti-Hallucination Guards](#prompt-engineering--anti-hallucination-guards)
   - [Token Optimization & Robust JSON Recovery](#token-optimization--robust-json-recovery)
7. [Error Handling & Production Resilience](#7-error-handling--production-resilience)
8. [Testing & Verification Suite](#8-testing--verification-suite)

---

## 1. Overview & Challenge Theme

> *"Build anything that needs an answer it can't afford to get wrong."*

APIs serve as permanent public contracts. Once an endpoint is published to production clients—mobile applications, third-party developers, and partner SDKs—breaking design changes become virtually impossible to roll back without extensive versioning debt. 

**Proto** was built to solve the API design verification problem at the developer's terminal before any code is deployed.

### Why Standard Approaches Fail
1. **Raw LLM Hallucinations:** When prompted with nuanced API questions, general-purpose LLMs frequently hallucinate non-existent AIP numbers, fabricate RFC rules, or offer conflicting, opinion-based advice.
2. **Keyword Search Limitations:** Searching a raw documentation corpus with an input like `curl -X POST https://api.example.com/getUser -d '{"id":"123"}'` yields zero matches because the command contains none of the actual keywords (*"safe methods"*, *"idempotency"*, *"RFC 9110 § 9.3.1"*, *"AIP-131 standard methods"*).

### How Proto Solves It
Proto couples terminal-native developer workflows directly with **Sanity Context MCP**, which indexes the **RESTICE API Knowledge Base** (Google API Improvement Proposals, RFC 9110 HTTP Semantics, Zalando RESTful API Guidelines, and RFC 7807/9457 Problem Details). Every answer, finding, and suggested design produced by Proto is grounded in structured, source-linked documentation.

---

## 2. Architecture & System Flow

```text
                       DEVELOPER TERMINAL
                               │
                ┌──────────────┴──────────────┐
                ▼                             ▼
       Natural Language Input          Raw Curl Command
        ("Should I use PUT...")       ("curl -X POST ...")
                │                             │
                ▼                             ▼
         [agent.Research]               [agent.Review]
                │                             │
                ▼                             ▼
       Phase 2 Research Plan           Phase 1 Intent Parser
       (2-4 targeted queries)         (method, path, headers, body)
                │                             │
                └──────────────┬──────────────┘
                               ▼
                    Sanity Context MCP Client
                               │
            ┌──────────────────┼──────────────────┐
            ▼                  ▼                  ▼
     initial_context     knowledge_base_    knowledge_base_
      (schema outline)        search             read
                               │                  │
                               ▼                  ▼
                       RESTICE Knowledge Base (Sanity)
                       (AIPs, RFC 9110, Zalando Guidelines)
                               │
                               ▼
                    Normalized Research Context
                               │
                               ▼
                     OpenRouter LLM Engine
                   (google/gemini-2.5-flash)
                               │
                               ▼
                   Structured Terminal Output
            (Answer / Issues, Citations & Source Links)
```

---

## 3. Phase 1: Terminal API Design Reviewer

### Phase 1 Scope & Objectives
The goal of Phase 1 was to create a lightweight CLI tool capable of inspecting raw `curl` commands, identifying anti-patterns, and recommending resource-oriented designs grounded in Sanity knowledge.

### Curl Parser (`parser.go`)
The curl parser converts shell command strings into a structured [`APIRequest`](file:///workspaces/proto/models.go#L44):
```go
type APIRequest struct {
    Method  string            `json:"method"`
    URL     string            `json:"url"`
    Path    string            `json:"path"`
    Headers map[string]string `json:"headers"`
    Body    string            `json:"body"`
}
```
Key parsing capabilities:
- **Flag Support:** Handles `-X`, `--request`, `-H`, `--header`, `-d`, `--data`, `--data-raw`, and `--url`.
- **Quote Awareness:** Accurately tokenizes single quotes (`'...'`), escaped quotes (`'\''`), and double quotes (`"..."`).
- **Implicit HTTP Verbs:** Automatically infers `POST` when payload flags (`-d`) are present without explicit `-X`; defaults to `GET` otherwise.
- **URI Path Extraction:** Automatically parses URI schemes and isolates the resource path (`/getUser`, `/v1/users/{id}`).

### Sanity Context MCP Client (`mcp.go`)
Proto connects to the hosted Sanity Context MCP endpoint via JSON-RPC 2.0 over Streamable HTTP:
1. `initialize`: Handshakes with the server using MCP protocol version `2024-11-05`.
2. `notifications/initialized`: Confirms session initialization.
3. `tools/list`: Discovers available server capabilities.
4. `Search(ctx, query)`: Queries Sanity Context tools for relevant API design standards.

### API Review Engine & Output Format
For curl commands, Proto analyzes the request semantics, retrieves applicable AIP entries, and formats the output into structured findings:
```text
╭──────────────────────────────────────────╮
│                 PROTO                    │
│          API Design Research Agent       │
╰──────────────────────────────────────────╯

▸ Parsing API...
  ✓ POST /getUser

▸ Understanding API...
  ✓ Retrieval operation detected using a non-standard method.

▸ Querying Sanity Context...
  Sanity Context
       ↓
  4 relevant entries retrieved
       ↓
  4 source documents
       ↓
  • HTTP Semantics & Method Properties
  • Custom Method Design, Validation & Job Patterns
  • Standard CRUD Methods
  • Resource-Oriented Design: Principles & Patterns

  ✓ Knowledge retrieved from Sanity

▸ Consulting API guidance...
  ✓ Relevant guidance found

▸ Reviewing API...
──────────────────────────────────────────

⚠ FINDING 1

POST /getUser

The API is designed to retrieve a user by ID, which is a read-only operation.
HTTP POST methods are generally used for creating resources or executing operations
with side effects. Using POST for a read operation goes against the defined semantics
of HTTP methods.

Relevant guidance:
  HTTP Semantics & Method Properties
  AIP-131: Standard methods: Get

Why:
  Using POST for a read operation.

Suggested design:
  Change the HTTP method from POST to GET. For standard Get methods, the identifier
  should be part of the URL path (e.g., /users/{id}).

──────────────────────────────────────────

Sources:
  • HTTP Semantics & Method Properties
  • AIP-131
  • AIP-121

──────────────────────────────────────────

Proto found 3 issues.
```

### Phase 1 Verified Scenarios
1. **Demo 1 — Retrieval Anti-pattern (`POST /getUser`):**
   - Command: `go run . review 'curl -X POST https://api.example.com/getUser -d "{\"id\":\"123\"}"'`
   - Detection: Flags that POST is unsafe/non-idempotent for reads, flags prohibited request bodies on GET, and flags RPC verb naming.
   - Recommended Design: `GET /v1/users/{user}`.
2. **Demo 2 — Sub-property Mutation Anti-pattern (`PUT /users/123/email`):**
   - Command: `go run . review 'curl -X PUT https://api.example.com/users/123/email -d "{\"email\":\"x@example.com\"}"'`
   - Detection: Flags that PUT implies complete resource replacement, and exposing individual properties as sub-resources fragments the resource model.
   - Recommended Design: `PATCH /v1/users/{user}` with a field mask or partial body.
3. **Demo 3 — Clean Design Conformance (`GET /users/123`):**
   - Command: `go run . review 'curl https://api.example.com/users/123'`
   - Output: `✓ API design conforms to standard guidance. No issues found.`

---

## 4. Phase 2: Natural Language API Design Research Agent

### Phase 2 Scope & Objectives
Phase 2 expanded Proto from a curl-only linter into a full **API design research agent**. Developers can ask complex, open-ended questions about HTTP semantics, resource hierarchies, design trade-offs, and AIP standards.

### The Five-Stage Research Workflow

```text
User Question
     │
     ▼
Stage 1: Understand
     ├─ Classifies intent (Comparison, Review, Explanation, Design, Debugging)
     │
     ▼
Stage 2: Research Plan
     ├─ Formulates 2-4 targeted research questions
     │
     ▼
Stage 3: Query Sanity Context MCP
     ├─ Executes dynamic BM25 search & entry reading for each question
     ├─ Aggregates & deduplicates retrieved entries
     │
     ▼
Stage 4: Normalize Research Context
     ├─ Compiles compact evidence context with document provenance
     │
     ▼
Stage 5: Final Reasoning & Synthesis
     ├─ OpenRouter model reasons strictly over retrieved evidence
     │
     ▼
Terminal Output
     └─ Emits structured summary, detailed answer, guidance citations & sources
```

### Stage 1 & 2: Intent Classification & Research Planning
The LLM inspects the user question and returns a [`ResearchPlan`](file:///workspaces/proto/models.go#L4):
- **Question Types:**
  - `API design comparison` (e.g. PUT vs. PATCH)
  - `API design review` (e.g. evaluating an endpoint structure)
  - `API concept explanation` (e.g. explaining AIP-131 to junior engineers)
  - `Resource-oriented API design` (e.g. designing full entity collections)
  - `API debugging & trade-offs` (e.g. large query parameters vs. POST bodies)
- **Targeted Questions:** 2 to 4 precise research topics specifically tailored for Sanity Knowledge Base retrieval.

### Stage 3: Dynamic Multi-Query MCP Retrieval
Proto iterates over each research question in the plan and executes searches against Sanity:
1. `extractKeywords`: Sanitizes natural-language queries into high-density BM25 search keywords (e.g. stripping stop words, capturing AIP numbers, preserving HTTP verbs).
2. `knowledge_base_search`: Queries the indexed knowledge base `kbzijGjZZuE3` and retrieves ranked entry paths.
3. `knowledge_base_read`: Fetches full markdown bodies with source citations.
4. `parseKnowledgeEntries`: Splits multi-entry payloads into discrete documents, extracting titles and original web URLs.

### Stage 4 & 5: Normalized Context & Grounded Synthesis
Proto normalizes all entries into a clean markdown evidence document:
```text
[Entry 1: Standard CRUD Methods]
Source: AIP-134 (google.aip.dev/134)
...
[Entry 2: HTTP Semantics & Method Properties]
Source: RFC 9110 (HTTP Semantics)
...
```
The final reasoning prompt enforces strict rules:
- Prefer retrieved Sanity evidence over internal weights.
- Never invent standards or fabricate citations.
- Explicitly cite retrieved documentation for factual claims.
- Acknowledge when evidence is insufficient.

### Phase 2 Verified Scenarios

#### Scenario 1: Comparison (PUT vs. PATCH)
```bash
go run . "Should I use PUT or PATCH when updating a user's email?"
```
- **Research Plan Generated:**
  1. What are the semantics and differences of HTTP PUT vs PATCH according to RFC 9110?
  2. How does AIP-134 specify standard Update methods and partial updates?
  3. What are the idempotency considerations for PUT vs PATCH when updating a single field?
- **Sanity Documents Consulted:** `HTTP Semantics & Method Properties`, `Standard CRUD Methods`, `Idempotency & Retries`.
- **Finding:** Strongly recommends `PATCH /v1/users/{id}` for partial updates; explains that `PUT` semantically replaces the entire resource and risks clearing omitted fields.

#### Scenario 2: Explanation (AIP-131)
```bash
go run . "Explain AIP-131 to me like I'm a junior backend engineer."
```
- **Research Plan Generated:**
  1. What is the primary purpose and scope of AIP-131?
  2. What are standard method conventions for Get operations?
- **Sanity Documents Consulted:** `Standard CRUD Methods`, `Resource-Oriented Design: Principles & Patterns`, `HTTP Semantics`.
- **Finding:** Explains safe and idempotent read operations, URL naming conventions (`/v1/{name=publishers/*/books/*}`), prohibited request bodies, and the 200/404 lifecycle.

#### Scenario 3: Review (POST /getUser)
```bash
go run . "Is POST /getUser a reasonable API design?"
```
- **Research Plan Generated:**
  1. What are the standard HTTP methods for retrieving resources according to HTTP Semantics (RFC 9110)?
  2. How does AIP-131 recommend designing methods for retrieving resources?
  3. What are the implications of using POST for a read-only operation regarding idempotency and caching?
- **Sanity Documents Consulted:** `Standard CRUD Methods (AIP-131)`, `HTTP Semantics & Method Properties (RFC 9110)`, `Resource-Oriented Design (AIP-121)`.
- **Finding:** Explains why `POST /getUser` violates HTTP method semantics, breaks caching and retries, and proposes `GET /v1/users/{user}`. Explains the rare exception (query filters exceeding URL length limits).

#### Scenario 4: Resource Design (Projects & Members)
```bash
go run . "Design an API for creating projects and managing project members."
```
- **Research Plan Generated:**
  1. How does AIP-121 define resource hierarchies for parent-child relationships?
  2. What are standard CRUD patterns for resource creation and deletion?
  3. How should sub-resources or related resources be modeled (AIP-124)?
- **Sanity Documents Consulted:** `Resource-Oriented Design (AIP-121)`, `Resource Names (AIP-122)`, `Pagination (AIP-158)`, `One Team Owns Each Type (AIP-2713)`.
- **Design Output:**
  ```http
  POST   /v1/projects                       # Create project
  GET    /v1/projects                       # List projects (paginated)
  GET    /v1/projects/{project}             # Get project details
  PATCH  /v1/projects/{project}             # Update project
  DELETE /v1/projects/{project}             # Delete project

  POST   /v1/projects/{project}/members     # Add member
  GET    /v1/projects/{project}/members     # List members
  GET    /v1/projects/{project}/members/{m} # Get member
  PATCH  /v1/projects/{project}/members/{m} # Update member role
  DELETE /v1/projects/{project}/members/{m} # Remove member
  ```

---

## 5. Sanity Context MCP Integration Deep-Dive

### Protocol & Transport Layer
Proto interfaces directly with Sanity Context using **Model Context Protocol (MCP)**:
- **Transport:** HTTP POST with dual Accept headers (`Accept: application/json, text/event-stream`).
- **Authentication:** `Authorization: Bearer <SANITY_CONTEXT_TOKEN>`.
- **Session Continuity:** Captures and forwards `Mcp-Session-Id` response headers.

### Tool Orchestration Sequence
```text
1. initialize
   └── Client handshake with protocolVersion: "2024-11-05"

2. notifications/initialized
   └── Session confirmation notification

3. tools/list
   └── Discovers available tools:
       • initial_context
       • knowledge_base_search
       • knowledge_base_read

4. tools/call: initial_context
   └── Inspects overview, extracts Knowledge Base ID (`kbzijGjZZuE3`)

5. tools/call: knowledge_base_search
   └── Parameters: {"knowledgeBase": "kbzijGjZZuE3", "query": "...", "return": "paths", "limit": 4}
   └── Returns ranked entry paths with BM25 scores

6. tools/call: knowledge_base_read
   └── Parameters: {"knowledgeBase": "kbzijGjZZuE3", "paths": [...]}
   └── Retrieves full markdown documentation with source URLs
```

### Why Structured Knowledge Bases Matter
Unlike an unstructured vector database that performs naive chunk similarity, Sanity's Knowledge Base provides:
1. **Curated Outlines:** High-level topical taxonomy ensuring the agent can orient itself before retrieving details.
2. **Explicit Cross-References:** Links related concepts (e.g. AIP-134 linking to AIP-121 and RFC 9110).
3. **Verified Provenance:** Every claim retains its original web citation (e.g. `https://google.aip.dev/131`, `https://opensource.zalando.com/restful-api-guidelines`).

---

## 6. OpenRouter LLM Integration Deep-Dive

### Prompt Engineering & Anti-Hallucination Guards
All prompts ([`prompt.go`](file:///workspaces/proto/prompt.go)) enforce strict evidence boundaries:
- **Zero Fabrication:** The agent is explicitly prohibited from generating hypothetical standard numbers.
- **Differentiating Guidance from Interpretation:** The model must distinguish explicit documented requirements ("MUST use GET") from design recommendations ("PATCH is preferred").
- **Trade-off Awareness:** Discourages dogmatic rejections; teaches the model to explain when an unconventional design is a legitimate trade-off (e.g. POST-based search filters).

### Token Optimization & Robust JSON Recovery
OpenRouter requests are configured with safeguards:
1. **Dynamic MaxTokens Budget:** Sets `max_tokens: 3500` to prevent token cutoff while operating within rate and credit limits.
2. **JSON Extraction & Repair:**
   - Cleans markdown fences (````json ... ````).
   - If an LLM response is truncated near the end, [`repairOrExtractAnswer`](file:///workspaces/proto/llm.go#L271) dynamically attempts bracket closure or regex extraction of `"summary"` and `"answer"`, ensuring raw JSON syntax never leaks to the terminal.

---

## 7. Error Handling & Production Resilience

Proto converts complex backend errors into clean, human-readable terminal alerts:

| Condition | Terminal Output |
| :--- | :--- |
| **Missing MCP URL** | `✗ SANITY_CONTEXT_MCP_URL is not configured.` |
| **Missing LLM Key** | `✗ OPENROUTER_API_KEY is not configured.` |
| **MCP Connection Failure** | `✗ Could not connect to Sanity Context.`<br>`Check SANITY_CONTEXT_MCP_URL and SANITY_CONTEXT_TOKEN.` |
| **Empty Retrieval** | `⚠ No sufficiently relevant knowledge was found.`<br>`Proto will answer only from the available evidence and clearly identify the limitation.` |
| **LLM Generation Error** | `✗ Unable to generate the final answer.` |
| **Invalid Curl Syntax** | `✗ Could not understand the curl command.`<br>`Proto currently supports: curl URL, curl -X METHOD URL, curl -H HEADER, curl -d BODY` |

Stack traces, authentication tokens, and raw HTTP headers are never exposed to the terminal.

---

## 8. Testing & Verification Suite

Proto includes comprehensive automated unit and integration tests:

- **Curl Parser Tests ([`parser_test.go`](file:///workspaces/proto/parser_test.go)):** Tests simple GET, explicit POST, PATCH with headers/body, multiline curl commands, implicit method detection, and malformed inputs.
- **MCP Integration Tests ([`mcp_test.go`](file:///workspaces/proto/mcp_test.go)):** Spins up an `httptest.Server` simulating the Sanity MCP JSON-RPC protocol to test handshakes, tool listing, tool calling, and authentication rejections.
- **LLM Client Tests ([`llm_test.go`](file:///workspaces/proto/llm_test.go)):** Spins up an `httptest.Server` simulating OpenRouter completions to verify research planning, answer synthesis, and curl review parsing.

To execute the test suite:
```bash
go test -v ./...
```
*(All tests execute in ~0.02s).*
