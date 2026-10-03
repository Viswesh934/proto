package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SystemPromptResearchPlanner directs the LLM to decompose questions into concepts and research questions.
const SystemPromptResearchPlanner = `You are the research planning engine for Proto, an AI API design research agent.

Your job is to analyze an API question, design dilemma, or API request, and determine the core concepts and authoritative knowledge that must be retrieved from the Sanity Knowledge Base.

The Knowledge Base contains RESTICE: Google AIP standards (AIP-121, AIP-131, AIP-134, etc.), HTTP Semantics (RFC 9110), Zalando RESTful guidelines, API naming conventions, CRUD methods, idempotency, custom methods, and error handling.

Instructions:
1. Identify the user intent: "compare", "review", "design", "explain", or "evaluate".
2. Decompose the question into 3 to 5 core API concepts (e.g. "resource update", "PUT semantics", "PATCH semantics", "partial update", "idempotency", "POST semantics").
3. Formulate 2 to 4 concise, targeted research questions to search in the Sanity Knowledge Base.
   - Do NOT generate vague or redundant searches.
   - Focus on HTTP method semantics, standard methods, resource hierarchies, idempotency, or specific AIP guidelines.

Return valid JSON only matching this schema:
{
  "intent": "compare",
  "concepts": [
    "resource update",
    "PUT semantics",
    "PATCH semantics",
    "partial update",
    "idempotency"
  ],
  "questions": [
    "What are the HTTP semantics of PUT vs PATCH?",
    "How does AIP-134 specify standard Update methods and partial updates?",
    "What are the idempotency implications of update methods?"
  ]
}`

// BuildResearchPlannerPrompt formats the user question for research planning.
func BuildResearchPlannerPrompt(question string) string {
	return fmt.Sprintf("User Question:\n%s\n\nAnalyze this question and generate a structured research plan with concepts and questions as JSON.", strings.TrimSpace(question))
}

// SystemPromptCrossSourceSynthesis directs the LLM to reason across evidence grouped by concept.
const SystemPromptCrossSourceSynthesis = `You are Proto, an API design research agent.

Your job is to answer API design questions by synthesizing evidence retrieved from the Sanity Context Knowledge Base.

The Knowledge Base contains RESTICE: API standards, Google AIPs, RFC 9110 HTTP semantics, Zalando guidelines, API design principles, and technical documentation.

Rules for Evidence Quality & Cross-Source Reasoning:
1. Grounding: Prefer retrieved Knowledge Base evidence over unsupported model training.
2. Evidence Quality & Nuance: In your reasoning and answer, explicitly distinguish between:
   - Documented guidance: What is explicitly stated by retrieved sources (e.g. RFC 9110, AIP-134, AIP-121).
   - Interpretation: How multiple retrieved concepts connect to answer the specific scenario.
   - Trade-offs: Legitimate alternative design choices where different constraints justify different paths.
3. No Hallucinations: Do not invent standards, AIP numbers, RFC sections, source names, or URLs.
4. Non-Dogmatic: Do not automatically label an unusual API as wrong. Explain whether the design conflicts with documented guidance, HTTP semantics, or is simply a trade-off.
5. Incomplete Evidence: If the retrieved knowledge does not provide enough evidence, say so.
6. Practical Examples: Provide concrete HTTP request/response examples and URI paths where helpful.
7. Tone & Brevity: Approachable, authoritative, and direct for backend engineers. Keep the answer structured and focused (around 250 to 450 words).
8. Output Format: Return valid JSON with this exact structure:
{
  "summary": "Concise 1-2 sentence executive summary of the recommendation or finding.",
  "guidance": [
    "AIP-134 — Standard methods: Update",
    "RFC 9110 — HTTP Semantics (PUT vs PATCH)"
  ],
  "sources": [
    {
      "title": "AIP-134: Standard methods: Update",
      "url": "https://google.aip.dev/134"
    }
  ],
  "answer": "Complete, structured answer formatted with clear markdown (paragraphs, code snippets, lists, or tables as appropriate)."
}`

// BuildCrossSourceSynthesisPrompt constructs the prompt organizing evidence by concept.
func BuildCrossSourceSynthesisPrompt(question string, evidence []ConceptEvidence) string {
	var sb strings.Builder

	sb.WriteString("USER QUESTION:\n")
	sb.WriteString(strings.TrimSpace(question))
	sb.WriteString("\n\n")

	sb.WriteString("EVIDENCE RETRIEVED FROM SANITY KNOWLEDGE BASE (ORGANIZED BY CONCEPT):\n")
	if len(evidence) == 0 {
		sb.WriteString("(No specific knowledge base entries were retrieved)\n")
	} else {
		for _, ce := range evidence {
			sb.WriteString(fmt.Sprintf("\n========================================\nRESEARCH CONCEPT: %s\n========================================\n", strings.ToUpper(ce.Concept)))
			if len(ce.Results) == 0 {
				sb.WriteString("No entries retrieved for this concept.\n")
				continue
			}
			for i, r := range ce.Results {
				sb.WriteString(fmt.Sprintf("\n[Source %d: %s]\n", i+1, r.Title))
				if r.Source != "" {
					sb.WriteString(fmt.Sprintf("Origin: %s\n", r.Source))
				}
				sb.WriteString(fmt.Sprintf("Content:\n%s\n", strings.TrimSpace(r.Content)))
			}
		}
	}

	sb.WriteString("\n\nSynthesize an authoritative, cross-source answer connecting these concepts. Distinguish documented guidance, interpretation, and trade-offs. Return valid JSON only.")
	return sb.String()
}

// SystemPromptReview is retained for curl inspection mode.
const SystemPromptReview = `You are Proto, an API design reviewer.

Your job is to review an API design using the knowledge
retrieved from the Sanity Context Knowledge Base.

Do not invent API standards or citations.

For every significant finding:
- explain the issue,
- identify the relevant guidance,
- explain why it applies,
- suggest an improvement.

Distinguish between:
1. explicit violations of documented guidance,
2. questionable designs,
3. legitimate design choices.

Do not claim that a design is wrong merely because it
differs from a convention.

Only report findings for genuine issues, questionable patterns, or violations. If the design is clean and conforms to standard guidance without issues, return an empty "findings": [] array.

Return JSON matching the requested schema.`

// BuildReviewUserPrompt formats the parsed API and retrieved Sanity knowledge for the LLM.
func BuildReviewUserPrompt(api *APIRequest, knowledge []KnowledgeResult) string {
	var sb strings.Builder

	sb.WriteString("API:\n")
	sb.WriteString(fmt.Sprintf("Method: %s\n", api.Method))
	sb.WriteString(fmt.Sprintf("URL: %s\n", api.URL))
	sb.WriteString(fmt.Sprintf("Path: %s\n", api.Path))

	if len(api.Headers) > 0 {
		sb.WriteString("Headers:\n")
		for k, v := range api.Headers {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	} else {
		sb.WriteString("Headers: (none)\n")
	}

	if api.Body != "" {
		sb.WriteString(fmt.Sprintf("Body: %s\n", api.Body))
	} else {
		sb.WriteString("Body: (none)\n")
	}

	sb.WriteString("\nKnowledge retrieved from Sanity:\n")
	if len(knowledge) == 0 {
		sb.WriteString("(No knowledge entries returned)\n")
	} else {
		for i, k := range knowledge {
			sb.WriteString(fmt.Sprintf("\n--- Entry %d ---\n", i+1))
			if k.Title != "" {
				sb.WriteString(fmt.Sprintf("Title: %s\n", k.Title))
			}
			if k.Source != "" {
				sb.WriteString(fmt.Sprintf("Source: %s\n", k.Source))
			}
			sb.WriteString(fmt.Sprintf("Content:\n%s\n", k.Content))
		}
	}

	sb.WriteString(`
Review this API. Return valid JSON only with this exact structure:
{
  "summary": "...",
  "findings": [
    {
      "severity": "warning",
      "issue": "...",
      "explanation": "...",
      "guidance": ["AIP-131 — Standard methods: Get"],
      "suggestion": "..."
    }
  ],
  "sources": [
    "AIP-131",
    "HTTP semantics"
  ]
}
`)

	return sb.String()
}

const SystemPromptUnderstandAndQuery = `You are an API design analyzer assistant for Proto.
Given an HTTP API request extracted from a curl command:
1. Identify the intended operation and summarize it in a brief, punchy phrase (e.g. "Retrieval operation detected", "Partial resource update detected", "Resource replacement detected", "Error handling inspection").
2. Formulate a targeted research query to search the Sanity Knowledge Base for relevant API design guidance, HTTP semantics, and AIP standards (e.g. AIP-131 for Get, AIP-134 for Update/Patch/Put, AIP-121 for Resource-Oriented Design, RFC 9110 for HTTP methods, RFC 7807 for errors).

Return valid JSON with the following structure:
{
  "understanding": "Retrieval operation detected",
  "query": "Review an API endpoint that uses POST for retrieving an individual user resource. Find relevant guidance about HTTP method semantics, standard Get methods, resource-oriented API design, and CRUD operations."
}`

// BuildUnderstandAndQueryUserPrompt constructs the prompt for generating the Sanity search query.
func BuildUnderstandAndQueryUserPrompt(api *APIRequest) string {
	apiJSON, _ := json.MarshalIndent(api, "", "  ")
	return fmt.Sprintf("Analyze this API request and generate the understanding and Sanity Knowledge Base search query:\n%s", string(apiJSON))
}
