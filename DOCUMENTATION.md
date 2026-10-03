# Proto: Complete Technical Documentation (Phase 1, Phase 2 & Phase 3)

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
5. [Phase 3: Concept-Grounded Synthesis & Native CLI Experience](#5-phase-3-concept-grounded-synthesis--native-cli-experience)
   - [Phase 3 Objectives & Evolution](#phase-3-objectives--evolution)
   - [Concept Decomposition Engine](#concept-decomposition-engine)
   - [In-Memory Evidence Graph (`ConceptEvidence`)](#in-memory-evidence-graph-conceptevidence)
   - [Cross-Source Synthesis: Guidance, Interpretation & Trade-offs](#cross-source-synthesis-guidance-interpretation--trade-offs)
   - [Token Budgeting & Context Optimization](#token-budgeting--context-optimization)
   - [Native `proto` CLI & Interactive REPL](#native-proto-cli--interactive-repl)
   - [Phase 3 Verified Scenarios & Live Transcripts](#phase-3-verified-scenarios--live-transcripts)
6. [Sanity Context MCP Integration Deep-Dive](#6-sanity-context-mcp-integration-deep-dive)
   - [Protocol & Transport Layer](#protocol--transport-layer)
   - [Tool Orchestration Sequence](#tool-orchestration-sequence)
   - [Why Structured Knowledge Bases Matter](#why-structured-knowledge-bases-matter)
7. [OpenRouter LLM Integration Deep-Dive](#7-openrouter-llm-integration-deep-dive)
   - [Prompt Engineering & Anti-Hallucination Guards](#prompt-engineering--anti-hallucination-guards)
   - [Token Optimization & Robust JSON Recovery](#token-optimization--robust-json-recovery)
8. [Error Handling & Production Resilience](#8-error-handling--production-resilience)
9. [Testing & Verification Suite](#9-testing--verification-suite)

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

## 5. Phase 3: Concept-Grounded Synthesis & Native CLI Experience

### Phase 3 Objectives & Evolution
While Phase 2 proved that an agent could dynamically formulate research queries and retrieve Sanity knowledge, it treated retrieved documents as a flat string dump (`researchContext`) passed into the LLM. 

Phase 3 upgrades Proto into a **concept-driven synthesis agent**:
1. **Concept Decomposition:** Rather than searching for the literal user question, Proto identifies the underlying architectural concepts (e.g., *resource update*, *PUT semantics*, *PATCH semantics*, *partial updates*, *idempotency*).
2. **In-Memory Evidence Graph:** Retrieved entries are structured into [`ConceptEvidence`](file:///workspaces/proto/models.go#L11-L15) structs, grouping authoritative guidance by concept.
3. **Cross-Source Synthesis:** Reasoning explicitly differentiates between:
   - **Documented guidance:** Hard mandates directly specified in sources (e.g. RFC 9110, AIP-134, AIP-121).
   - **Interpretation:** How multiple standards combine to address the user's specific context.
   - **Trade-offs:** Legitimate alternative choices based on differing constraints.
4. **Native `proto` CLI Experience:** Replaces `go run . "question"` with an installed, globally available binary (`proto`) supporting both single-command execution and an interactive REPL session.

```text
                    ┌────────────────────────┐
                    │       proto CLI        │
                    │ (interactive / direct) │
                    └───────────┬────────────┘
                                │
                                ▼
                    ┌────────────────────────┐
                    │  Concept Decomposition │
                    │   (PlanResearch LLM)   │
                    └───────────┬────────────┘
                                │
                ┌───────────────┼───────────────┐
                ▼               ▼               ▼
           Concept 1       Concept 2       Concept 3
         (PUT Semantics) (PATCH Semantics) (Idempotency)
                │               │               │
                └───────────────┼───────────────┘
                                ▼
                    Concurrent Sanity MCP
                    (Targeted Search & Read)
                                │
                                ▼
                    ┌────────────────────────┐
                    │ Concept Evidence Graph │
                    │   ([]ConceptEvidence)  │
                    └───────────┬────────────┘
                                │
                                ▼
                    ┌────────────────────────┐
                    │ Cross-Source Reasoning │
                    │   (SynthesizeAnswer)   │
                    └───────────┬────────────┘
                                │
              ┌─────────────────┼─────────────────┐
              ▼                 ▼                 ▼
          Documented      Interpretation      Trade-offs
           Guidance
```

---

### Concept Decomposition Engine
When given a user query, Proto's planning engine ([`PlanResearch`](file:///workspaces/proto/llm.go#L83)) extracts the user intent, 3–5 core API concepts, and 2–4 targeted research questions:

```json
{
  "intent": "design",
  "concepts": [
    "resource update",
    "partial update",
    "HTTP methods",
    "PUT semantics",
    "PATCH semantics",
    "POST semantics",
    "idempotency"
  ],
  "questions": [
    "What are the HTTP semantics for PUT, PATCH, and POST methods according to RFC 9110?",
    "What are the idempotency characteristics of PUT, PATCH, and POST methods?",
    "How does AIP-134 (Standard Update Method) guide the design of partial updates?"
  ]
}
```

Queries are executed **concurrently** against Sanity Context MCP using goroutines, reducing multi-query retrieval latency from ~6 seconds to ~1.5 seconds.

---

### In-Memory Evidence Graph (`ConceptEvidence`)
Retrieved knowledge results are organized into conceptual clusters ([`models.go`](file:///workspaces/proto/models.go)):

```go
type ConceptEvidence struct {
    Concept string            `json:"concept"`
    Results []KnowledgeResult `json:"results"`
}
```

The evidence graph engine:
1. Maps query-specific knowledge results directly to their parent concept.
2. Performs keyword-based association across all retrieved entries to enrich concepts.
3. Implements **global deduplication** so the same source document is never duplicated verbatim across multiple concepts.
4. Truncates individual entries to concise, high-signal excerpts (max 1,500 characters) to optimize prompt token density.

---

### Cross-Source Synthesis: Guidance, Interpretation & Trade-offs
In [`prompt.go`](file:///workspaces/proto/prompt.go), `SystemPromptCrossSourceSynthesis` instructs the model to structure its analysis across three evidentiary dimensions:

1. **Documented Guidance:** What is explicitly written in standards (e.g., RFC 9110 § 9.3.4 defines `PUT` as a complete state replacement; AIP-134 specifies that `PATCH` is strongly preferred for updates).
2. **Interpretation:** How those rules apply to the specific question (e.g., updating only an email address modifies a single attribute of the user resource, making `PATCH /v1/users/{user}` the canonical solution).
3. **Trade-offs:** Where different constraints permit different architectures (e.g., `PUT` requires sending the entire resource representation; `POST` breaks idempotency and standard caching; dedicated sub-resources like `/users/123/email` cause URI fragmentation unless email has an independent lifecycle).

---

### Token Budgeting & Context Optimization
To prevent token budget overflow and preserve LLM reasoning headroom:
1. **Deduplicated Content Budget:** Global deduplication limits prompt size to ~3,000–4,000 tokens regardless of how many concepts are active.
2. **Pre-buffered Schema Fields:** The JSON output schema defines `"summary"`, `"guidance"`, and `"sources"` **before** `"answer"`. Even if an answer approaches token ceilings, authoritative citations are preserved.
3. **Resilient JSON Recovery:** [`repairOrExtractAnswer`](file:///workspaces/proto/llm.go#L277) uses multi-pass closing brackets and regular expression fallback to recover structured answers without leaking syntax errors.

---

### Native `proto` CLI & Interactive REPL
Phase 3 introduces a first-class command-line interface ([`cli.go`](file:///workspaces/proto/cli.go)):

1. **Interactive REPL Session (`proto`):**
   - Continuously prompts the developer with `> `.
   - Accepts questions or curl commands.
   - Preserves state across multiple inquiries.
   - Cleanly exits on `exit`, `quit`, or `Ctrl+D` (EOF).
2. **Direct Execution (`proto "<question>"`):**
   - Directly executes the research pipeline for a single question and exits with code 0.
3. **Curl Review (`proto review '<curl>'`):**
   - Parses curl commands, detects anti-patterns, and suggests resource-oriented alternatives.
4. **Help & Discovery (`proto --help`):**
   - Displays clear command usage, flags, and representative examples.

---

### Phase 3 Verified Scenarios & Live Transcripts

#### Scenario 1: Primary Demo — PUT vs PATCH vs POST for Partial Update
**Command:**
```bash
proto "I need to update only a user's email address. Should my API use PUT, PATCH, or POST?"
```
**Terminal Output:**
```text
╭────────────────────────────────────────────╮
│                  PROTO                     │
│          API Design Research Agent         │
╰────────────────────────────────────────────╯

Question
> I need to update only a user's email address. Should my API
> use PUT, PATCH, or POST?

◆ Understanding question...

  Intent: design

◆ Building research plan...

  • resource update
  • partial update
  • HTTP methods
  • PUT semantics
  • PATCH semantics
  • POST semantics
  • idempotency

◆ Querying Sanity Context...

  ✓ What are the HTTP semantics for PUT, PATCH, and POST methods according to RFC 9110?
  ✓ What are the idempotency characteristics of PUT, PATCH, and POST methods?
  ✓ How does AIP-134 (Standard Update Method) guide the design of partial updates?
  ✓ When is POST an appropriate method for resource updates versus PUT or PATCH?

  Sanity Context
       ↓
  16 relevant entries retrieved
       ↓
  7 source documents
       ↓
  • HTTP Semantics & Method Properties
  • Standard CRUD Methods
  • Custom Method Design, Validation & Job Patterns
  • Error Handling & Status Codes
  • Idempotency & Retries
  • Resource-Oriented Design: Principles & Patterns
  • API Governance, Design Review & AIP Process

  ✓ Knowledge retrieved from Sanity

◆ Connecting evidence...

  ✓ Linked 7 concepts across 7 authoritative sources

◆ Reasoning...

  ✓ Cross-source analysis complete

──────────────────────────────────────────

ANSWER

To update only a user's email address, `PATCH` is the most semantically appropriate HTTP method. `PATCH` is designed for partial modifications, allowing clients to send only the fields that need to be changed. While `PUT` can also update, it typically requires sending the entire resource representation, and `POST` is generally reserved for creating resources or custom actions.

When updating only a specific field, such as a user's email address, the choice between `PUT`, `PATCH`, and `POST` depends on the desired semantics and behavior:

1.  PATCH (Recommended for partial updates):
    `PATCH` is specifically designed for applying partial modifications to a resource. This means the client sends only the fields that are intended to be updated, rather than the full resource representation. For updating just a user's email, `PATCH` is the most semantically accurate choice.
    - Example:
      PATCH /users/123
      Content-Type: application/json
      {"email": "new.email@example.com"}

2.  PUT (Alternative for full resource replacement):
    `PUT` is used to replace an entire resource. If you use `PUT` to update a user's email, the expectation is that the request body contains the complete representation. Any omitted fields would typically be reset or removed.

3.  POST (Generally for creation or custom actions):
    `POST` is neither safe nor idempotent. Using `POST` for a simple field update deviates from standard RESTful principles where `PATCH` is specifically designed for this purpose.

──────────────────────────────────────────

RELEVANT GUIDANCE

• AIP-134 — Standard methods: Update
• RFC 9110 — HTTP Semantics (PUT vs PATCH)
• AIP-130 — Standard CRUD Methods
• AIP-121 — Resource-Oriented Design: Principles & Patterns
• AIP-136 — Custom Method Design, Validation & Job Patterns

──────────────────────────────────────────

SOURCES

• HTTP Semantics & Method Properties (https://opensource.zalando.com/restful-api-guidelines/#http-requests)
• AIP-130: Methods (https://google.aip.dev/130)
• AIP-121: Resource-oriented design (https://google.aip.dev/121)
• AIP-136: Custom methods (https://google.aip.dev/136)

──────────────────────────────────────────
```

#### Scenario 2: Endpoint Evaluation — `POST /getUser`
**Command:**
```bash
proto "Is POST /getUser a reasonable API design?"
```
**Outcome:** Proto explains why `POST /getUser` is an anti-pattern under RFC 9110 and AIP-131, detailing loss of cacheability and lack of idempotency guarantees, and recommends `GET /users/{id}`.

#### Scenario 3: Explanatory Inquiry — Junior Backend Guide to AIP-131
**Command:**
```bash
proto "Explain AIP-131 to me like I'm a junior backend engineer."
```
**Outcome:** Explains the core purpose of AIP-131 (retrieving a single resource), why `GET` is safe and idempotent, URI hierarchy conventions (`/v1/{name=publishers/*/books/*}`), prohibited request bodies, and provides a clear proto/HTTP definition.

#### Scenario 4: Resource Design — Projects & Member Sub-collections
**Command:**
```bash
proto "Design an API for creating projects, updating projects, and managing project members."
```
**Outcome:** Models `projects` as a top-level collection and `members` as a nested sub-collection (`/v1/projects/{project}/members`), specifying standard `POST`, `GET`, `PATCH` (with field masks), and `DELETE` endpoints, alongside idempotency key guidance (AIP-155).

#### Scenario 5: Curl Command Review
**Command:**
```bash
proto review 'curl -X POST https://api.example.com/getUser -H "Content-Type: application/json" -d "{\"id\":\"123\"}"'
```
**Outcome:** Detects 4 issues (POST for retrieval, URI naming convention, prohibited request body in GET, and RPC camelCase naming) and suggests `GET /v1/users/{id}`.

#### Scenario 6: Interactive Terminal REPL
**Session:**
```text
$ proto
╭────────────────────────────────────────────╮
│                  PROTO                     │
│          API Design Research Agent         │
╰────────────────────────────────────────────╯

Ask an API design question (or 'exit' to quit):
> Should I use PUT or PATCH when updating a user's email?

... [Executes research and prints structured answer] ...

Ask an API design question (or 'exit' to quit):
> exit
Goodbye!
```

---

## 6. Sanity Context MCP Integration Deep-Dive

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
