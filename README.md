# Proto — API Design Research Agent

> *"Build anything that needs an answer it can't afford to get wrong."*
>
> APIs are public contracts. Once an endpoint is published to mobile apps, SDKs, and third-party developers, design mistakes are prohibitively expensive to fix. **Proto** is an AI API design research agent powered by **Sanity Context MCP**. Instead of relying purely on an LLM's pretrained memory or hallucinations, Proto dynamically formulates a research plan, queries the Sanity Knowledge Base (containing authoritative Google AIPs, RFC 9110 HTTP semantics, and Zalando guidelines), and synthesizes source-grounded answers directly in your terminal.

📖 **Full Technical Documentation:** See [DOCUMENTATION.md](DOCUMENTATION.md) for detailed Phase 1 & Phase 2 architecture, MCP tool orchestration, and prompt engineering.

```text
                       USER
                        │
                        ▼
                 Natural language (or curl)
                        │
                        ▼
                 ┌──────────────┐
                 │    PROTO     │
                 │    Agent     │
                 └──────┬───────┘
                        │
                        ▼
                 Understand question
                        │
                        ▼
                  Research plan
                        │
                        ▼
              Sanity Context MCP
                        │
                        ▼
               Knowledge Base (RESTICE)
                        │
                 ┌──────┴──────┐
                 ▼             ▼
              Entries        Sources
                 │             │
                 └──────┬──────┘
                        ▼
                  OpenRouter LLM
                        │
                        ▼
                Source-backed answer
                        │
                        ▼
                    Terminal
```

There is **no frontend** — the terminal is the complete product and demo interface.

---

## Why Proto Needs Structured Sanity Context

### The "Can't Afford to Get It Wrong" Problem
When designing or reviewing APIs:
1. **Raw LLMs hallucinate:** They invent non-existent RFC numbers, fabricate AIP standards, or provide conflicting advice based on generic web crawls.
2. **Keyword search fails:** A question like *"Is POST /getUser a reasonable API design?"* contains none of the keywords like *"safe methods"*, *"idempotency"*, *"RFC 9110 § 9.3.1"*, *"AIP-131 standard methods"*, or *"resource-oriented URI hierarchy"*. A keyword query against raw documentation produces useless noise.

### Why Proto Only Works Because Content is Structured
- **Authoritative & Curated:** Sanity compiles verified API standards into structured, interlinked entries with source provenance.
- **Navigable Through MCP:** Proto dynamically navigates the Knowledge Base outline (`initial_context`), discovers entry paths through ranked BM25 search (`knowledge_base_search`), and reads full cited markdown documentation (`knowledge_base_read`).
- **Grounded Evidence:** Proto strictly attributes facts to retrieved documents and acknowledges when guidance is absent rather than guessing.

---

## Agent Workflow

Proto executes a five-stage terminal pipeline:

1. **Stage 1 — Understand:** Analyzes the question and classifies the intent (*API design comparison*, *API design review*, *API concept explanation*, *Resource-oriented API design*, or *API debugging & trade-offs*).
2. **Stage 2 — Research Plan:** Formulates a targeted 2–4 question research plan targeting specific standards and HTTP mechanics.
3. **Stage 3 — Query Sanity Context MCP:** Executes queries against the live Sanity Knowledge Base, discovering and reading authoritative guidance entries.
4. **Stage 4 — Research Summary:** Normalizes retrieved entries into a compact, cited research context.
5. **Stage 5 — Final Reasoning:** Synthesizes the final answer using OpenRouter with strict instructions prohibiting ungrounded claims or invented citations.

---

## Terminal Experience

The terminal makes the research and Sanity interaction visible:

```text
╭──────────────────────────────────────────╮
│                 PROTO                    │
│          API Design Research Agent       │
╰──────────────────────────────────────────╯

Question
> Should I use PUT or PATCH when updating a user's email?

◆ Understanding question...

  Type: API design comparison

◆ Building research plan...

  1. What are the semantics and differences of HTTP PUT vs PATCH according to RFC 9110?
  2. How does AIP-134 specify standard Update methods and partial updates?
  3. What are the idempotency considerations for PUT vs PATCH when updating a single field?

◆ Querying Sanity Context...

  ✓ What are the semantics and differences of HTTP PUT vs PATCH according to RFC 9110?
  ✓ How does AIP-134 specify standard Update methods and partial updates?
  ✓ What are the idempotency considerations for PUT vs PATCH when updating a single field?

  Sanity Context
       ↓
  9 relevant entries retrieved
       ↓
  9 source documents
       ↓
  • HTTP Semantics & Method Properties
  • Standard CRUD Methods
  • Resource-Oriented Design: Principles & Patterns
  • Idempotency & Retries

  ✓ Knowledge retrieved from Sanity

◆ Reasoning over retrieved knowledge...

  ✓ Analysis complete

──────────────────────────────────────────

ANSWER

When updating a user's email, `PATCH` is the strongly preferred HTTP method over `PUT`.

When updating a user's email, you should use the `PATCH` HTTP method. This is because
`PATCH` is designed for partial updates to a resource, which is what changing a single
field like an email address represents.

• PATCH for Partial Updates: Modifies only a subset of the resource state.
  AIP-134 states that PATCH is "strongly preferred over PUT" for update methods.

• PUT for Full Replacement: Replaces the entire resource state at the given URL.
  If fields are omitted in a PUT payload, they are reset or cleared.

```http
PATCH /v1/users/user-123
Content-Type: application/json

{
  "email": "new.email@example.com"
}
```

──────────────────────────────────────────

RELEVANT GUIDANCE

• AIP-134 — Standard methods: Update
• RFC 9110 — HTTP Semantics (PUT vs PATCH)

──────────────────────────────────────────

SOURCES

• AIP-134: Standard methods: Update (https://google.aip.dev/134)
• Zalando RESTful API Guidelines § 148 (HTTP Semantics & Method Properties)
```

---

## Demo Commands

### 1. Compare: PUT vs PATCH for Updating Resources
```bash
go run . "Should I use PUT or PATCH when updating a user's email?"
```

### 2. Review: Evaluating Endpoints
```bash
go run . "Is POST /getUser a reasonable API design?"
```

### 3. Design: Resource Hierarchy & Sub-resources
```bash
go run . "Design an API for creating projects and managing project members."
```

### 4. Explain: Plain-English Standard Explanations
```bash
go run . "Explain AIP-131 to me like I'm a junior backend engineer."
```

### 5. Curl / API Input (Direct Review)
```bash
go run . 'curl -X POST https://api.example.com/getUser -d "{\"id\":\"123\"}"'
```

### 6. Interactive Mode
```bash
go run .
```

---

## Configuration

Credentials are loaded from environment variables or a local `.env` file (see [`.env.example`](.env.example)):

```env
SANITY_CONTEXT_MCP_URL=https://api.sanity.io/v1/context/organizations/.../mcp/...
SANITY_CONTEXT_TOKEN=sk...

OPENROUTER_API_KEY=sk-or-v1-...
OPENROUTER_MODEL=google/gemini-2.5-flash
```

*(Offline fallback: Setting `SANITY_CONTEXT_MCP_URL=mock` and `OPENROUTER_API_KEY=mock` enables a local mock engine for testing without external services).*

---

## Running Verification Tests

```bash
go test -v ./...
```