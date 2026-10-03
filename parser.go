package main

import (
	"errors"
	"net/url"
	"strings"
)

var ErrInvalidCurl = errors.New("Could not understand the curl command.\n\nProto currently supports:\n  curl URL\n  curl -X METHOD URL\n  curl -H HEADER\n  curl -d BODY")

// ParseCurl parses a curl command string into an APIRequest.
func ParseCurl(cmd string) (*APIRequest, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil, ErrInvalidCurl
	}

	// Clean up line continuations: backslash followed by newline
	cmd = strings.ReplaceAll(cmd, "\\\r\n", " ")
	cmd = strings.ReplaceAll(cmd, "\\\n", " ")
	cmd = strings.ReplaceAll(cmd, "\r\n", " ")
	cmd = strings.ReplaceAll(cmd, "\n", " ")

	tokens, err := tokenizeCurl(cmd)
	if err != nil || len(tokens) == 0 {
		return nil, ErrInvalidCurl
	}

	req := &APIRequest{
		Headers: make(map[string]string),
	}

	explicitMethod := false
	hasData := false

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		// Skip "curl" prefix if present
		if i == 0 && strings.EqualFold(token, "curl") {
			continue
		}

		switch {
		case token == "-X" || token == "--request":
			if i+1 < len(tokens) {
				i++
				req.Method = strings.ToUpper(strings.TrimSpace(tokens[i]))
				explicitMethod = true
			}
		case strings.HasPrefix(token, "-X") && len(token) > 2:
			req.Method = strings.ToUpper(strings.TrimSpace(token[2:]))
			explicitMethod = true
		case token == "-H" || token == "--header":
			if i+1 < len(tokens) {
				i++
				addHeader(req.Headers, tokens[i])
			}
		case strings.HasPrefix(token, "--header="):
			addHeader(req.Headers, strings.TrimPrefix(token, "--header="))
		case token == "-d" || token == "--data" || token == "--data-raw" || token == "--data-ascii" || token == "--data-binary":
			if i+1 < len(tokens) {
				i++
				req.Body = tokens[i]
				hasData = true
			}
		case strings.HasPrefix(token, "--data="):
			req.Body = strings.TrimPrefix(token, "--data=")
			hasData = true
		case strings.HasPrefix(token, "--data-raw="):
			req.Body = strings.TrimPrefix(token, "--data-raw=")
			hasData = true
		case token == "--url":
			if i+1 < len(tokens) {
				i++
				req.URL = tokens[i]
			}
		case strings.HasPrefix(token, "--url="):
			req.URL = strings.TrimPrefix(token, "--url=")
		case !strings.HasPrefix(token, "-"):
			// First non-flag token is considered the URL if not already set
			if req.URL == "" && (strings.Contains(token, "/") || strings.Contains(token, ".") || strings.Contains(token, ":")) {
				req.URL = token
			}
		}
	}

	if req.URL == "" {
		return nil, ErrInvalidCurl
	}

	// Default HTTP method
	if !explicitMethod {
		if hasData {
			req.Method = "POST"
		} else {
			req.Method = "GET"
		}
	}

	// Extract path from URL
	req.Path = extractPath(req.URL)

	return req, nil
}

func addHeader(headers map[string]string, headerStr string) {
	headerStr = strings.TrimSpace(headerStr)
	parts := strings.SplitN(headerStr, ":", 2)
	if len(parts) == 2 {
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		headers[k] = v
	}
}

func extractPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil && u.Path != "" {
		return u.Path
	}
	// If missing scheme (e.g. api.example.com/getUser)
	if !strings.Contains(rawURL, "://") {
		u2, err2 := url.Parse("https://" + rawURL)
		if err2 == nil && u2.Path != "" {
			return u2.Path
		}
	}
	return "/"
}

// tokenizeCurl tokenizes a curl command while respecting single and double quotes.
func tokenizeCurl(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' && !inSingle {
			escaped = true
			continue
		}

		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}

		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if (r == ' ' || r == '\t') && !inSingle && !inDouble {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens, nil
}
