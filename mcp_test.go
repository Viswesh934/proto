package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSanityMCPClient_Integration(t *testing.T) {
	// Create a mock MCP server that speaks JSON-RPC 2.0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req mcpRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch req.Method {
		case "initialize":
			resp := mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: json.RawMessage(`{
					"protocolVersion": "2024-11-05",
					"capabilities": {},
					"serverInfo": {"name": "sanity-context", "version": "1.0.0"}
				}`),
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusOK)

		case "tools/list":
			resp := mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: json.RawMessage(`{
					"tools": [
						{
							"name": "knowledge_base_read",
							"description": "Reads knowledge base entries",
							"inputSchema": {"type": "object"}
						}
					]
				}`),
			}
			_ = json.NewEncoder(w).Encode(resp)

		case "tools/call":
			resp := mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: json.RawMessage(`{
					"content": [
						{
							"type": "text",
							"text": "AIP-131 standard GET method guidelines."
						}
					]
				}`),
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "Unknown method", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewSanityMCPClient(server.URL, "test-token")
	results, err := client.Search(context.Background(), "GET retrieval")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("Expected at least 1 result, got 0")
	}

	if results[0].Content != "AIP-131 standard GET method guidelines." {
		t.Errorf("Unexpected content: %s", results[0].Content)
	}
}

func TestSanityMCPClient_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewSanityMCPClient(server.URL, "bad-token")
	_, err := client.Search(context.Background(), "test")
	if err == nil {
		t.Fatal("Expected error for unauthorized request, got nil")
	}
}
