package main

import (
	"testing"
)

func TestParseCurl(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantMethod string
		wantURL    string
		wantPath   string
		wantBody   string
		wantHeader string
	}{
		{
			name:       "simple GET",
			cmd:        "curl https://api.example.com/users",
			wantMethod: "GET",
			wantURL:    "https://api.example.com/users",
			wantPath:   "/users",
		},
		{
			name:       "explicit POST without data",
			cmd:        "curl -X POST https://api.example.com/users",
			wantMethod: "POST",
			wantURL:    "https://api.example.com/users",
			wantPath:   "/users",
		},
		{
			name:       "PATCH with headers and body",
			cmd:        `curl -X PATCH https://api.example.com/users/123 -H "Content-Type: application/json" -d '{"name":"John"}'`,
			wantMethod: "PATCH",
			wantURL:    "https://api.example.com/users/123",
			wantPath:   "/users/123",
			wantBody:   `{"name":"John"}`,
			wantHeader: "application/json",
		},
		{
			name:       "POST getUser demo 1",
			cmd:        `curl -X POST https://api.example.com/getUser -H "Content-Type: application/json" -d '{"id":"123"}'`,
			wantMethod: "POST",
			wantURL:    "https://api.example.com/getUser",
			wantPath:   "/getUser",
			wantBody:   `{"id":"123"}`,
			wantHeader: "application/json",
		},
		{
			name:       "PUT user email demo 2",
			cmd:        `curl -X PUT https://api.example.com/users/123/email -d "{\"email\":\"x@example.com\"}"`,
			wantMethod: "PUT",
			wantURL:    "https://api.example.com/users/123/email",
			wantPath:   "/users/123/email",
			wantBody:   `{"email":"x@example.com"}`,
		},
		{
			name:       "implicit POST with -d",
			cmd:        `curl https://api.example.com/create -d "test"`,
			wantMethod: "POST",
			wantURL:    "https://api.example.com/create",
			wantPath:   "/create",
			wantBody:   "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := ParseCurl(tt.cmd)
			if err != nil {
				t.Fatalf("ParseCurl() error = %v", err)
			}
			if req.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", req.Method, tt.wantMethod)
			}
			if req.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", req.URL, tt.wantURL)
			}
			if req.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", req.Path, tt.wantPath)
			}
			if tt.wantBody != "" && req.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", req.Body, tt.wantBody)
			}
			if tt.wantHeader != "" && req.Headers["Content-Type"] != tt.wantHeader {
				t.Errorf("Content-Type = %q, want %q", req.Headers["Content-Type"], tt.wantHeader)
			}
		})
	}
}

func TestParseCurl_Invalid(t *testing.T) {
	invalidCmds := []string{
		"",
		"   ",
		"curl -X POST",
	}

	for _, cmd := range invalidCmds {
		_, err := ParseCurl(cmd)
		if err == nil {
			t.Errorf("ParseCurl(%q) expected error, got nil", cmd)
		}
	}
}
