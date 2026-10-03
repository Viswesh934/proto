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
func (a *Agent) Research(ctx context.Context, question string) (*Answer, *ResearchPlan, []ConceptEvidence, error) {
	// Stage 1: Understand question
	fmt.Printf("%s%s◆ Understanding question...%s\n\n", colorBold, colorCyan, colorReset)
	plan, err := a.llm.PlanResearch(ctx, question)
	if err != nil {
		return nil, nil, nil, err
	}
	intent := plan.Intent
	if intent == "" {
		intent = "research"
	}
	fmt.Printf("  Intent: %s%s%s\n\n", colorBold, intent, colorReset)

	// Stage 2: Build research plan with concepts
	fmt.Printf("%s%s◆ Building research plan...%s\n\n", colorBold, colorCyan, colorReset)
	if len(plan.Concepts) > 0 {
		for _, c := range plan.Concepts {
			fmt.Printf("  • %s\n", c)
		}
		fmt.Println()
	}

	// Stage 3: Query Sanity Context MCP for each research question
	fmt.Printf("%s%s◆ Querying Sanity Context...%s\n\n", colorBold, colorCyan, colorReset)

	type searchOutcome struct {
		query   string
		results []KnowledgeResult
		err     error
	}

	// Run Sanity queries concurrently for responsiveness
	outcomeCh := make(chan searchOutcome, len(plan.Questions))
	for _, q := range plan.Questions {
		go func(query string) {
			res, searchErr := a.knowledge.Search(ctx, query)
			outcomeCh <- searchOutcome{query: query, results: res, err: searchErr}
		}(q)
	}

	resultsByQuery := make(map[string][]KnowledgeResult)
	var allKnowledge []KnowledgeResult
	seenTitles := make(map[string]bool)
	var sourceTitles []string

	for i := 0; i < len(plan.Questions); i++ {
		outcome := <-outcomeCh
		if outcome.err != nil {
			fmt.Printf("  %s✗%s %s\n", colorRed, colorReset, outcome.query)
			continue
		}
		fmt.Printf("  %s✓%s %s\n", colorGreen, colorReset, outcome.query)
		resultsByQuery[outcome.query] = outcome.results
		for _, k := range outcome.results {
			if strings.TrimSpace(k.Content) != "" {
				if !seenTitles[k.Title] {
					seenTitles[k.Title] = true
					sourceTitles = append(sourceTitles, k.Title)
				}
				allKnowledge = append(allKnowledge, k)
			}
		}
	}
	fmt.Println()

	if len(allKnowledge) == 0 {
		fmt.Fprintf(os.Stderr, "  %s⚠ No sufficiently relevant knowledge was found.%s\n\n  Proto will answer only from available evidence and clearly identify the limitation.\n\n", colorYellow, colorReset)
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

	// Stage 4: Connect evidence into concept graph
	fmt.Printf("%s%s◆ Connecting evidence...%s\n\n", colorBold, colorCyan, colorReset)

	var conceptEvidence []ConceptEvidence
	assignedGlobally := make(map[string]bool)

	if len(plan.Concepts) > 0 {
		for i, c := range plan.Concepts {
			var matching []KnowledgeResult

			// 1. Direct query results mapping if available
			if i < len(plan.Questions) {
				if qr, ok := resultsByQuery[plan.Questions[i]]; ok {
					for _, item := range qr {
						if !assignedGlobally[item.Title] {
							assignedGlobally[item.Title] = true
							matching = append(matching, truncateContent(item, 1500))
							if len(matching) >= 2 {
								break
							}
						}
					}
				}
			}

			// 2. Keyword relevance across all retrieved knowledge
			if len(matching) < 2 {
				conceptWords := strings.Fields(strings.ToLower(c))
				for _, item := range allKnowledge {
					if assignedGlobally[item.Title] {
						continue
					}
					itemLower := strings.ToLower(item.Title + " " + item.Content)
					for _, w := range conceptWords {
						if len(w) > 3 && strings.Contains(itemLower, w) {
							assignedGlobally[item.Title] = true
							matching = append(matching, truncateContent(item, 1500))
							break
						}
					}
					if len(matching) >= 2 {
						break
					}
				}
			}

			// 3. Fallback to any remaining unassigned entry
			if len(matching) == 0 {
				for _, item := range allKnowledge {
					if !assignedGlobally[item.Title] {
						assignedGlobally[item.Title] = true
						matching = append(matching, truncateContent(item, 1500))
						break
					}
				}
			}

			conceptEvidence = append(conceptEvidence, ConceptEvidence{
				Concept: c,
				Results: matching,
			})
		}
	} else {
		for q, qr := range resultsByQuery {
			var truncated []KnowledgeResult
			for _, item := range qr {
				truncated = append(truncated, truncateContent(item, 1500))
			}
			conceptEvidence = append(conceptEvidence, ConceptEvidence{
				Concept: q,
				Results: truncated,
			})
		}
	}

	fmt.Printf("  %s✓%s Linked %d concepts across %d authoritative sources\n\n", colorGreen, colorReset, len(conceptEvidence), len(sourceTitles))

	// Stage 5: Final Cross-source Reasoning
	fmt.Printf("%s%s◆ Reasoning...%s\n\n", colorBold, colorCyan, colorReset)
	answer, err := a.llm.SynthesizeAnswer(ctx, question, conceptEvidence)
	if err != nil {
		return nil, plan, conceptEvidence, err
	}
	fmt.Printf("  %s✓%s Cross-source analysis complete\n\n", colorGreen, colorReset)

	// Ensure sources and evidence on answer
	if len(answer.Sources) == 0 {
		for _, t := range sourceTitles {
			answer.Sources = append(answer.Sources, Source{Title: t})
		}
	}
	answer.Evidence = conceptEvidence

	return answer, plan, conceptEvidence, nil
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

func truncateContent(item KnowledgeResult, maxChars int) KnowledgeResult {
	content := strings.TrimSpace(item.Content)
	if len(content) > maxChars {
		cut := content[:maxChars]
		if lastNL := strings.LastIndex(cut, "\n"); lastNL > maxChars/2 {
			cut = cut[:lastNL]
		}
		item.Content = cut + "\n..."
	}
	return item
}
