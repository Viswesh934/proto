package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOpenRouterClient_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-openrouter-key" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req openRouterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// Case 1: Research Planner
		if len(req.Messages) > 0 && req.Messages[0].Content == SystemPromptResearchPlanner {
			resp := openRouterResponse{
				Choices: []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				}{
					{
						Message: struct {
							Content string `json:"content"`
						}{
							Content: `{"question_type": "API design comparison", "questions": ["PUT semantics", "PATCH semantics"]}`,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Case 2: Cross-Source Synthesis
		if len(req.Messages) > 0 && req.Messages[0].Content == SystemPromptCrossSourceSynthesis {
			resp := openRouterResponse{
				Choices: []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				}{
					{
						Message: struct {
							Content string `json:"content"`
						}{
							Content: `{"summary": "Use PATCH for email", "answer": "Detailed answer", "guidance": ["AIP-134"], "sources": [{"title": "AIP-134"}]}`,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Case 3: Understand and query
		if len(req.Messages) > 0 && req.Messages[0].Content == SystemPromptUnderstandAndQuery {
			resp := openRouterResponse{
				Choices: []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				}{
					{
						Message: struct {
							Content string `json:"content"`
						}{
							Content: `{"understanding": "Retrieval operation detected", "query": "AIP-131 GET"}`,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Case 4: Review
		resp := openRouterResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: "```json\n{\n  \"summary\": \"Problematic POST\",\n  \"findings\": [{\"severity\": \"warning\", \"issue\": \"POST for get\", \"explanation\": \"Retrieve resource\", \"guidance\": [\"AIP-131\"], \"suggestion\": \"GET /v1/users/{id}\"}],\n  \"sources\": [\"AIP-131\"]\n}\n```",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	os.Setenv("OPENROUTER_BASE_URL", server.URL)
	defer os.Unsetenv("OPENROUTER_BASE_URL")

	client := NewOpenRouterClient("test-openrouter-key", "test-model")

	// Test PlanResearch
	plan, err := client.PlanResearch(context.Background(), "Should I use PUT or PATCH when updating a user's email?")
	if err != nil {
		t.Fatalf("PlanResearch failed: %v", err)
	}
	if plan.QuestionType != "API design comparison" {
		t.Errorf("Unexpected question type: %s", plan.QuestionType)
	}
	if len(plan.Questions) != 2 {
		t.Errorf("Expected 2 questions, got %d", len(plan.Questions))
	}

	// Test SynthesizeAnswer
	evidence := []ConceptEvidence{
		{
			Concept: "partial updates",
			Results: []KnowledgeResult{
				{Title: "AIP-134", Content: "Update methods", Source: "google.aip.dev"},
			},
		},
	}
	ans, err := client.SynthesizeAnswer(context.Background(), "Should I use PUT or PATCH?", evidence)
	if err != nil {
		t.Fatalf("SynthesizeAnswer failed: %v", err)
	}
	if ans.Summary != "Use PATCH for email" {
		t.Errorf("Unexpected answer summary: %s", ans.Summary)
	}

	// Test Backward compatible curl Review
	api := &APIRequest{
		Method: "POST",
		URL:    "https://api.example.com/getUser",
		Path:   "/getUser",
	}

	understanding, query, err := client.UnderstandAndQuery(context.Background(), api)
	if err != nil {
		t.Fatalf("UnderstandAndQuery failed: %v", err)
	}
	if understanding != "Retrieval operation detected" {
		t.Errorf("Unexpected understanding: %s", understanding)
	}
	if query != "AIP-131 GET" {
		t.Errorf("Unexpected query: %s", query)
	}

	knowledge := []KnowledgeResult{
		{
			Title:   "AIP-131",
			Content: "Standard Get",
			Source:  "google.aip.dev",
		},
	}

	result, err := client.Review(context.Background(), api, knowledge)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	if len(result.Findings) != 1 {
		t.Fatalf("Expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Suggestion != "GET /v1/users/{id}" {
		t.Errorf("Unexpected suggestion: %s", result.Findings[0].Suggestion)
	}
}
