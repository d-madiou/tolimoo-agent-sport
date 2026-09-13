// Command research-check performs one bounded, developer-facing Exa search.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	appconfig "newsroom/internal/config"
	"newsroom/internal/providers/exa"
)

func main() {
	query := flag.String("query", "", "research query")
	flag.Parse()
	if strings.TrimSpace(*query) == "" {
		*query = strings.TrimSpace(strings.Join(flag.Args(), " "))
	}
	if strings.TrimSpace(*query) == "" {
		fmt.Fprintln(os.Stderr, "usage: research-check -query 'Premier League latest developments'")
		os.Exit(2)
	}
	if _, err := appconfig.LoadDotEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "load configuration:", err)
		os.Exit(1)
	}
	apiKey := os.Getenv("EXA_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		fmt.Fprintln(os.Stderr, "EXA_API_KEY is required for research-check")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := exa.New(apiKey, nil)
	client.MaxAttempts = 1 // one bounded developer smoke request; no paid retry.
	sources, err := client.Search(ctx, *query, 3)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Exa search failed:", err)
		os.Exit(1)
	}
	for index, source := range sources {
		publication := "unknown"
		if source.PublishedAt != nil {
			publication = source.PublishedAt.UTC().Format(time.RFC3339)
		}
		fmt.Printf("%d. %s\n   URL: %s\n   Published: %s\n   Text bytes: %d\n", index+1, source.Title, source.URL, publication, len(source.Content))
	}
	if len(sources) == 0 {
		fmt.Println("No sources returned.")
	}
}
