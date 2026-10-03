# Proto — API Design Research Agent

> *"Build anything that needs an answer it can't afford to get wrong."*
>
> APIs are permanent public contracts. Once an endpoint is published to mobile apps, SDKs, and third-party developers, design mistakes are prohibitively expensive to fix. **Proto** is an AI API design research agent powered by **Sanity Context MCP**. Instead of relying purely on an LLM's pretrained memory or hallucinations, Proto dynamically decomposes design questions into core API concepts, queries the Sanity Knowledge Base (containing authoritative Google AIPs, RFC 9110 HTTP semantics, and Zalando guidelines), builds an in-memory evidence graph, and synthesizes source-grounded answers directly in your terminal.

📖 **Full Technical Documentation:** See [DOCUMENTATION.md](DOCUMENTATION.md) for architecture, MCP tool orchestration, concept evidence graphs, and cross-source reasoning.

```text
                       USER
                        │
                        ▼
                 Proto CLI (proto)
            (interactive REPL or direct)
                        │
                        ▼
                 ┌──────────────┐
                 │    PROTO     │
                 │    Agent     │
                 └──────┬───────┘
                        │
                        ▼
                 Understand intent
                        │
                        ▼
               Decompose concepts
         (e.g., PUT vs PATCH, idempotency)
                        │
                        ▼
               Sanity Context MCP
         (concurrent targeted queries)
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
               Evidence / Concept Graph
          (RFC 9110, AIP-134, AIP-121)
                        │
                        ▼
                 OpenRouter LLM
         (Cross-source synthesis engine)
                        │
                        ▼
              Source-backed Terminal
       (Documented guidance, interpretation, trade-offs)
```

There is **no frontend** — the terminal is the complete product and demo interface.

---

## Why Proto Needs Structured Sanity Context

### The "Can't Afford to Get It Wrong" Problem
When designing or reviewing APIs:
1. **Raw LLMs hallucinate:** They invent non-existent RFC numbers, fabricate AIP standards, or provide conflicting advice based on generic web crawls.
2. **Keyword search fails:** A question like *"Is POST /getUser a reasonable API design?"* contains none of the keywords like *"safe methods"*, *"idempotency"*, *"RFC 9110 § 9.3.1"*, *"AIP-131 standard methods"*, or *"resource-oriented URI hierarchy"*. A keyword query against raw documentation produces useless noise.

### Why Proto Only Works Because Content is Structured
- **Concept-Oriented Retrieval:** Proto decomposes API dilemmas into distinct conceptual queries (HTTP method semantics, standard CRUD methods, idempotency).
- **Navigable Through MCP:** Proto dynamically navigates the Knowledge Base outline (`initial_context`), discovers entry paths through ranked BM25 search (`knowledge_base_search`), and reads full cited markdown documentation (`knowledge_base_read`).
- **Cross-Source Grounding:** Proto connects retrieved evidence into a concept graph, explicitly distinguishing between:
  - **Documented guidance:** Explicit mandates from RFC 9110, AIP-134, AIP-121, etc.
  - **Interpretation:** How standards synthesize to solve the specific developer scenario.
  - **Trade-offs:** Legitimate design alternatives and constraint trade-offs.

---

## Installation & CLI Setup

Proto is compiled into a single static binary:

```bash
# Build proto binary
go build -o proto .

# Install to user PATH (e.g., ~/.local/bin)
cp proto ~/.local/bin/proto
```

Verify installation:
```bash
proto --help
```

---

## Terminal Experience

Proto executes a five-stage terminal pipeline:

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

---

## CLI Commands & Modes

### 1. Interactive REPL Mode
Run `proto` with no arguments to launch the interactive research loop:

```bash
proto
```

```text
Ask an API design question (or 'exit' to quit):
> Should I use PUT or PATCH when updating a user's email?
...
Ask an API design question (or 'exit' to quit):
> exit
Goodbye!
```

### 2. Direct Question Mode
```bash
proto "I need to update only a user's email address. Should my API use PUT, PATCH, or POST?"
```

```bash
proto "Is POST /getUser a reasonable API design?"
```

```bash
proto "Design an API for creating projects, updating projects, and managing project members."
```

```bash
proto "Explain AIP-131 to me like I'm a junior backend engineer."
```

### 3. Curl / API Review Mode
Proto inspects raw `curl` commands and flags anti-patterns against Sanity guidance:

```bash
proto review 'curl -X POST https://api.example.com/getUser -H "Content-Type: application/json" -d "{\"id\":\"123\"}"'
```

---

## Configuration

Configure credentials via `.env` or system environment variables (see [`.env.example`](.env.example)):

```env
SANITY_CONTEXT_MCP_URL=https://api.sanity.io/v1/context/organizations/.../mcp/...
SANITY_CONTEXT_TOKEN=sk...

OPENROUTER_API_KEY=sk-or-v1-...
OPENROUTER_MODEL=google/gemini-2.5-flash
```

*(Offline fallback: Setting `SANITY_CONTEXT_MCP_URL=mock` and `OPENROUTER_API_KEY=mock` activates a deterministic mock engine for testing without network credentials).*

---

## Running Verification Tests

```bash
go test -v ./...
```
*(Tests complete in ~0.02s).*