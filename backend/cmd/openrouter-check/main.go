// Command openrouter-check performs one isolated, fictional OpenRouter draft check.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	appconfig "newsroom/internal/config"
	"newsroom/internal/domain"
	"newsroom/internal/providers/openrouter"
)

const (
	demoSourceID = "demo_source_1"
	demoLabel    = "DÉMO — INFORMATION FICTIVE"
)

func main() {
	if _, err := appconfig.LoadDotEnv(); err != nil {
		fail("load configuration", err)
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL")
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(model) == "" {
		fail("configuration", fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL are required"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := openrouter.New(apiKey, model, nil)
	result, err := client.Draft(ctx,
		"Rédige une publication Facebook et X en français sur ce match d'entraînement. Chaque texte et le titre doivent inclure exactement « DÉMO — INFORMATION FICTIVE » afin de signaler clairement qu'il ne s'agit pas d'une information réelle.",
		[]domain.SourceEvidence{{
			ID:          demoSourceID,
			Title:       "Fictional training-match announcement",
			URL:         "https://example.com/fictional-demo",
			Content:     "DÉMO — INFORMATION FICTIVE : le Club Alpha fictif programme un match d'entraînement contre le Club Beta fictif. Cette annonce est uniquement un scénario de démonstration et ne décrit aucun événement réel.",
			RetrievedAt: time.Now().UTC(),
		}},
		[]domain.StoryHistory{},
	)
	if err != nil {
		fail("OpenRouter draft check", err)
	}
	if err := validate(result); err != nil {
		fail("validate OpenRouter response", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail("encode validated response", err)
	}
	fmt.Println(string(data))
}

func validate(result domain.FinalOutput) error {
	if result.Outcome != "drafts" {
		return fmt.Errorf("outcome must be drafts")
	}
	if len(result.Drafts) < 1 || len(result.Drafts) > 2 {
		return fmt.Errorf("expected one or two drafts")
	}
	for index, draft := range result.Drafts {
		if strings.TrimSpace(draft.Headline) == "" || strings.TrimSpace(draft.FacebookText) == "" || strings.TrimSpace(draft.XText) == "" {
			return fmt.Errorf("draft %d has an empty required text field", index+1)
		}
		if draft.ClaimStatus == "official" {
			return fmt.Errorf("draft %d must not claim official evidence", index+1)
		}
		if utf8.RuneCountInString(draft.XText) > 280 {
			return fmt.Errorf("draft %d exceeds the 280-character X approximation", index+1)
		}
		for _, sourceID := range draft.SourceIDs {
			if sourceID != demoSourceID {
				return fmt.Errorf("draft %d references unknown source ID %q", index+1, sourceID)
			}
		}
		if !containsSource(draft.SourceIDs, demoSourceID) {
			return fmt.Errorf("draft %d does not reference %s", index+1, demoSourceID)
		}
		for _, sourceID := range draft.OfficialSourceIDs {
			if sourceID != demoSourceID {
				return fmt.Errorf("draft %d references unknown official source ID %q", index+1, sourceID)
			}
		}
		if !strings.Contains(draft.Headline, demoLabel) || !strings.Contains(draft.FacebookText, demoLabel) || !strings.Contains(draft.XText, demoLabel) {
			return fmt.Errorf("draft %d is missing the fictional-demo label", index+1)
		}
	}
	return nil
}

func containsSource(sourceIDs []string, want string) bool {
	for _, sourceID := range sourceIDs {
		if sourceID == want {
			return true
		}
	}
	return false
}

func fail(scope string, err error) {
	fmt.Fprintln(os.Stderr, scope+":", err)
	os.Exit(1)
}
