package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	cfg := loadConfig()

	if len(os.Args) >= 2 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		printUsage()
		return
	}

	// Validate environment configuration
	if cfg.SanityMCPURL == "" {
		fmt.Fprintf(os.Stderr, "%s✗ SANITY_CONTEXT_MCP_URL is not configured.%s\n", colorRed, colorReset)
		os.Exit(1)
	}

	if cfg.OpenRouterKey == "" {
		fmt.Fprintf(os.Stderr, "%s✗ OPENROUTER_API_KEY is not configured.%s\n", colorRed, colorReset)
		os.Exit(1)
	}

	agent := NewAgent(cfg)

	if len(os.Args) > 1 {
		RunDirectCommand(agent, strings.Join(os.Args[1:], " "))
	} else {
		StartInteractiveCLI(agent)
	}
}

// loadConfig loads environment variables and parses optional .env file.
func loadConfig() *Config {
	loadDotEnv(".env")

	return &Config{
		SanityMCPURL:    os.Getenv("SANITY_CONTEXT_MCP_URL"),
		SanityToken:     os.Getenv("SANITY_CONTEXT_TOKEN"),
		OpenRouterKey:   os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel: os.Getenv("OPENROUTER_MODEL"),
	}
}

func loadDotEnv(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
				v = v[1 : len(v)-1]
			}
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}
