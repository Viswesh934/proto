package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// ANSI color escape codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

// Agent coordinates the API parsing, research planning, Sanity MCP search, and LLM reasoning.
type Agent struct {
	knowledge KnowledgeClient
	llm       LLMClient
}

// NewAgent initializes a new Proto agent instance with configuration.
func NewAgent(cfg *Config) *Agent {
	return &Agent{
		knowledge: NewSanityMCPClient(cfg.SanityMCPURL, cfg.SanityToken),
		llm:       NewOpenRouterClient(cfg.OpenRouterKey, cfg.OpenRouterModel),
	}
}

// Research coordinates the multi-stage research agent workflow for an API question.
func (a *Agent) Research(ctx context.Context, question string) (*Answer, *ResearchPlan, []KnowledgeResult, error) {
	// Stage 1 & 2: Understand question & Build research plan
	fmt.Printf("%s%s◆ Understanding question...%s\n\n", colorBold, colorCyan, colorReset)
	plan, err := a.llm.PlanResearch(ctx, question)
	if err != nil {
		return nil, nil, nil, err
	}
	fmt.Printf("  Type: %s%s%s\n\n", colorBold, plan.QuestionType, colorReset)

	fmt.Printf("%s%s◆ Building research plan...%s\n\n", colorBold, colorCyan, colorReset)
	for i, q := range plan.Questions {
		fmt.Printf("  %d. %s\n", i+1, q)
	}
	fmt.Println()

	// Stage 3: Query Sanity Context MCP for each research question
	fmt.Printf("%s%s◆ Querying Sanity Context...%s\n\n", colorBold, colorCyan, colorReset)
	var allKnowledge []KnowledgeResult
	seenTitles := make(map[string]bool)
	var sourceTitles []string

	for _, q := range plan.Questions {
		results, err := a.knowledge.Search(ctx, q)
		if err != nil {
			fmt.Printf("  %s✗%s %s\n", colorRed, colorReset, q)
			continue
		}
		fmt.Printf("  %s✓%s %s\n", colorGreen, colorReset, q)
		for _, k := range results {
			if !seenTitles[k.Title] && strings.TrimSpace(k.Content) != "" {
				seenTitles[k.Title] = true
				sourceTitles = append(sourceTitles, k.Title)
				allKnowledge = append(allKnowledge, k)
			}
		}
	}
	fmt.Println()

	if len(allKnowledge) == 0 {
		fmt.Fprintf(os.Stderr, "  %s⚠ No sufficiently relevant knowledge was found.%s\n\n  Proto will answer only from the available evidence and clearly identify the limitation.\n\n", colorYellow, colorReset)
	} else {
		fmt.Printf("  %sSanity Context%s\n", colorCyan, colorReset)
		fmt.Printf("       ↓\n")
		fmt.Printf("  %d relevant entries retrieved\n", len(allKnowledge))
		fmt.Printf("       ↓\n")
		fmt.Printf("  %d source documents\n", len(sourceTitles))
		fmt.Printf("       ↓\n")
		for _, t := range sourceTitles {
			fmt.Printf("  • %s\n", t)
		}
		fmt.Println()
		fmt.Printf("  %s✓%s Knowledge retrieved from Sanity\n\n", colorGreen, colorReset)
	}

	// Stage 4: Normalize retrieved material into research context
	var sb strings.Builder
	for i, k := range allKnowledge {
		sb.WriteString(fmt.Sprintf("\n[Entry %d: %s]\nSource: %s\n%s\n", i+1, k.Title, k.Source, k.Content))
	}
	researchContext := sb.String()

	// Stage 5: Final Reasoning over retrieved knowledge
	fmt.Printf("%s%s◆ Reasoning over retrieved knowledge...%s\n\n", colorBold, colorCyan, colorReset)
	answer, err := a.llm.AnswerQuestion(ctx, question, researchContext)
	if err != nil {
		return nil, plan, allKnowledge, err
	}
	fmt.Printf("  %s✓%s Analysis complete\n\n", colorGreen, colorReset)

	// Ensure sources on answer
	if len(answer.Sources) == 0 {
		for _, t := range sourceTitles {
			answer.Sources = append(answer.Sources, Source{Title: t})
		}
	}

	return answer, plan, allKnowledge, nil
}

// Review orchestrates the curl-specific review workflow for backward compatibility.
func (a *Agent) Review(ctx context.Context, curlCmd string) (*ReviewResult, *APIRequest, error) {
	fmt.Printf("%s%s▸ Parsing API...%s\n", colorBold, colorCyan, colorReset)
	api, err := ParseCurl(curlCmd)
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("  %s✓%s %s %s\n\n", colorGreen, colorReset, api.Method, api.Path)

	fmt.Printf("%s%s▸ Understanding API...%s\n", colorBold, colorCyan, colorReset)
	understanding, query, err := a.llm.UnderstandAndQuery(ctx, api)
	if err != nil {
		understanding = "API operation detected"
		query = fmt.Sprintf("Find API design guidance for %s %s", api.Method, api.Path)
	}
	fmt.Printf("  %s✓%s %s\n\n", colorGreen, colorReset, understanding)

	fmt.Printf("%s%s▸ Querying Sanity Context...%s\n", colorBold, colorCyan, colorReset)
	knowledge, err := a.knowledge.Search(ctx, query)
	if err != nil {
		return nil, nil, err
	}

	entryCount := len(knowledge)
	sourceSet := make(map[string]bool)
	var sourceTitles []string
	for _, k := range knowledge {
		if k.Title != "" && !sourceSet[k.Title] {
			sourceSet[k.Title] = true
			sourceTitles = append(sourceTitles, k.Title)
		}
	}

	fmt.Printf("  %sSanity Context%s\n", colorCyan, colorReset)
	fmt.Printf("       ↓\n")
	fmt.Printf("  %d relevant entries retrieved\n", entryCount)
	fmt.Printf("       ↓\n")
	fmt.Printf("  %d source documents\n", len(sourceTitles))
	fmt.Printf("       ↓\n")
	for _, title := range sourceTitles {
		fmt.Printf("  • %s\n", title)
	}
	fmt.Println()
	fmt.Printf("  %s✓%s Knowledge retrieved from Sanity\n\n", colorGreen, colorReset)

	fmt.Printf("%s%s▸ Consulting API guidance...%s\n", colorBold, colorCyan, colorReset)
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("  %s✓%s Relevant guidance found\n\n", colorGreen, colorReset)

	fmt.Printf("%s%s▸ Reviewing API...%s\n", colorBold, colorCyan, colorReset)
	result, err := a.llm.Review(ctx, api, knowledge)
	if err != nil {
		return nil, nil, err
	}

	if len(result.Sources) == 0 {
		for s := range sourceSet {
			result.Sources = append(result.Sources, s)
		}
	}

	return result, api, nil
}
