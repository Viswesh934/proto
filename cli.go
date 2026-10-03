package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// StartInteractiveCLI launches the interactive REPL session.
func StartInteractiveCLI(agent *Agent) {
	printBanner()
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Println("Ask an API design question (or 'exit' to quit):")
		fmt.Print("> ")

		if !scanner.Scan() {
			fmt.Println("\nGoodbye!")
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") || strings.EqualFold(line, "q") {
			fmt.Println("Goodbye!")
			break
		}

		fmt.Println()
		runSingleInput(agent, line)
		fmt.Println()
	}
}

// RunDirectCommand executes a single question or curl command and exits.
func RunDirectCommand(agent *Agent, input string) {
	printBanner()
	runSingleInput(agent, input)
}

func runSingleInput(agent *Agent, rawInput string) {
	ctx := context.Background()

	// Handle review subcommand prefix if passed
	if strings.HasPrefix(rawInput, "review ") {
		rawInput = strings.TrimSpace(strings.TrimPrefix(rawInput, "review "))
	}

	if isCurlCommand(rawInput) {
		result, api, err := agent.Review(ctx, rawInput)
		if err != nil {
			printFormattedError(err)
			return
		}
		printReview(result, api)
		return
	}

	fmt.Printf("%sQuestion%s\n> %s\n\n", colorBold, colorReset, formatWrap(rawInput, 60))

	answer, _, _, err := agent.Research(ctx, rawInput)
	if err != nil {
		printFormattedError(err)
		return
	}

	printAnswer(answer)
}

func printBanner() {
	fmt.Printf("%s%s╭────────────────────────────────────────────╮%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s│                  PROTO                     │%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s│          API Design Research Agent         │%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s╰────────────────────────────────────────────╯%s\n\n", colorBold, colorCyan, colorReset)
}

func printUsage() {
	printBanner()
	fmt.Println("Usage:")
	fmt.Println("  proto \"<API design question>\"")
	fmt.Println("  proto review '<curl command>'")
	fmt.Println("  proto                           (interactive mode)")
	fmt.Println("  proto --help                    (display this help)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println(`  proto "Should I use PUT or PATCH when updating a user's email?"`)
	fmt.Println(`  proto "Explain AIP-131 to me like I'm a junior backend engineer."`)
	fmt.Println(`  proto "Is POST /getUser a reasonable API design?"`)
	fmt.Println(`  proto "Design an API for creating projects and managing project members."`)
	fmt.Println(`  proto review 'curl -X POST https://api.example.com/getUser -d "{\"id\":\"123\"}"'`)
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
	fmt.Println(divider)
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
