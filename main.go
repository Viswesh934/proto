package main

import (
	"bufio"
	"context"
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

	var rawInput string

	if len(os.Args) >= 2 && os.Args[1] == "review" {
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(1)
		}
		rawInput = strings.Join(os.Args[2:], " ")
	} else if len(os.Args) >= 2 {
		rawInput = strings.Join(os.Args[1:], " ")
	} else {
		// Interactive mode
		printBanner()
		fmt.Println("Ask an API design question or paste a curl command:")
		fmt.Print("> ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			rawInput = strings.TrimSpace(scanner.Text())
		}
		if rawInput == "" {
			return
		}
		fmt.Println()
	}

	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		fmt.Fprintf(os.Stderr, "%s✗ Proto could not understand the API question.%s\n", colorRed, colorReset)
		os.Exit(1)
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

	// If interactive mode didn't print banner yet, print it now
	if len(os.Args) > 1 {
		printBanner()
	}

	agent := NewAgent(cfg)
	ctx := context.Background()

	// Check if input is a pure curl command
	if isCurlCommand(rawInput) {
		result, api, err := agent.Review(ctx, rawInput)
		if err != nil {
			printFormattedError(err)
			os.Exit(1)
		}
		printReview(result, api)
		return
	}

	// Otherwise, run Phase 2 Research Agent workflow
	fmt.Printf("%sQuestion%s\n> %s\n\n", colorBold, colorReset, formatWrap(rawInput, 60))

	answer, _, _, err := agent.Research(ctx, rawInput)
	if err != nil {
		printFormattedError(err)
		os.Exit(1)
	}

	printAnswer(answer)
}

func printBanner() {
	fmt.Printf("%s%s╭──────────────────────────────────────────╮%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s│                 PROTO                    │%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s│          API Design Research Agent       │%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s╰──────────────────────────────────────────╯%s\n\n", colorBold, colorCyan, colorReset)
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  go run . \"<API design question>\"")
	fmt.Println("  go run . review '<curl command>'")
	fmt.Println("  go run .  (interactive mode)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println(`  go run . "Should I use PUT or PATCH when updating a user's email?"`)
	fmt.Println(`  go run . "Explain AIP-131 to me like I'm a junior backend engineer."`)
	fmt.Println(`  go run . "Is POST /getUser a reasonable API design?"`)
	fmt.Println(`  go run . "Design an API for creating projects and managing project members."`)
	fmt.Println(`  go run . review 'curl -X POST https://api.example.com/getUser -d "{\"id\":\"123\"}"'`)
}

func isCurlCommand(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "curl ") || strings.HasPrefix(trimmed, "curl\t") || strings.HasPrefix(trimmed, "curl\n")
}

func printAnswer(answer *Answer) {
	divider := "──────────────────────────────────────────"
	fmt.Println(divider)
	fmt.Println()
	fmt.Printf("%s%sANSWER%s\n\n", colorBold, colorCyan, colorReset)

	if answer.Summary != "" {
		fmt.Printf("%s%s%s\n\n", colorBold, answer.Summary, colorReset)
	}

	if answer.Answer != "" {
		fmt.Println(answer.Answer)
		fmt.Println()
	}

	if len(answer.Guidance) > 0 {
		fmt.Println(divider)
		fmt.Println()
		fmt.Printf("%s%sRELEVANT GUIDANCE%s\n\n", colorBold, colorCyan, colorReset)
		for _, g := range answer.Guidance {
			fmt.Printf("• %s\n", g)
		}
		fmt.Println()
	}

	if len(answer.Sources) > 0 {
		fmt.Println(divider)
		fmt.Println()
		fmt.Printf("%s%sSOURCES%s\n\n", colorBold, colorCyan, colorReset)
		for _, s := range answer.Sources {
			if s.URL != "" {
				fmt.Printf("• %s (%s)\n", s.Title, s.URL)
			} else {
				fmt.Printf("• %s\n", s.Title)
			}
		}
		fmt.Println()
	}
}

func printReview(result *ReviewResult, api *APIRequest) {
	divider := "──────────────────────────────────────────"
	fmt.Println(divider)

	var actualIssues []Finding
	for _, f := range result.Findings {
		suggLower := strings.ToLower(f.Suggestion)
		sevLower := strings.ToLower(f.Severity)
		if sevLower == "pass" || sevLower == "none" || strings.Contains(suggLower, "no change needed") || strings.Contains(suggLower, "no change required") {
			continue
		}
		actualIssues = append(actualIssues, f)
	}

	if len(actualIssues) == 0 {
		fmt.Printf("\n%s✓ API design conforms to standard guidance. No issues found.%s\n\n", colorGreen, colorReset)
	} else {
		for i, finding := range actualIssues {
			fmt.Println()
			if len(actualIssues) == 1 {
				fmt.Printf("%s%s⚠ API DESIGN ISSUE%s\n\n", colorBold, colorYellow, colorReset)
			} else {
				fmt.Printf("%s%s⚠ FINDING %d%s\n\n", colorBold, colorYellow, i+1, colorReset)
			}

			if api != nil {
				fmt.Printf("%s%s %s%s\n\n", colorBold, api.Method, api.Path, colorReset)
			}

			if finding.Explanation != "" {
				fmt.Printf("%s\n\n", finding.Explanation)
			} else if finding.Issue != "" {
				fmt.Printf("%s\n\n", finding.Issue)
			}

			if len(finding.Guidance) > 0 {
				fmt.Printf("%sRelevant guidance:%s\n", colorBold, colorReset)
				for _, g := range finding.Guidance {
					fmt.Printf("  %s\n", g)
				}
				fmt.Println()
			}

			if finding.Issue != "" && finding.Explanation != "" && finding.Issue != finding.Explanation {
				fmt.Printf("%sWhy:%s\n", colorBold, colorReset)
				fmt.Printf("  %s\n\n", finding.Issue)
			}

			if finding.Suggestion != "" {
				fmt.Printf("%sSuggested design:%s\n", colorBold, colorReset)
				fmt.Printf("  %s%s%s\n", colorGreen, finding.Suggestion, colorReset)
				fmt.Println()
			}
		}
	}

	fmt.Println(divider)

	if len(result.Sources) > 0 {
		fmt.Printf("\n%sSources:%s\n", colorBold, colorReset)
		for _, s := range result.Sources {
			fmt.Printf("  • %s\n", s)
		}
		fmt.Println()
		fmt.Println(divider)
	}

	fmt.Println()
	if len(actualIssues) == 1 {
		fmt.Println("Proto found 1 issue.")
	} else {
		fmt.Printf("Proto found %d issues.\n", len(actualIssues))
	}
}

func printFormattedError(err error) {
	errStr := err.Error()
	if strings.HasPrefix(errStr, "✗ ") {
		fmt.Fprintf(os.Stderr, "%s%s%s\n", colorRed, errStr, colorReset)
	} else {
		fmt.Fprintf(os.Stderr, "%s✗ %s%s\n", colorRed, errStr, colorReset)
	}
}

func formatWrap(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	words := strings.Fields(text)
	var lines []string
	var current strings.Builder

	for _, w := range words {
		if current.Len()+len(w)+1 > maxLen && current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(w)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return strings.Join(lines, "\n> ")
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
