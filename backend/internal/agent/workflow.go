// Package agent coordinates bounded research and draft generation.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"newsroom/internal/domain"
)

const (
	maxSearches   = 2
	maxModelCalls = 3
	maxDrafts     = 2
)

type Researcher interface {
	Search(context.Context, string, int) ([]domain.SourceEvidence, error)
}
type Writer interface {
	Assess(context.Context, string, []domain.SourceEvidence, []domain.StoryHistory) (domain.Assessment, error)
	Draft(context.Context, string, []domain.SourceEvidence, []domain.StoryHistory) (domain.FinalOutput, error)
}

type Workflow struct {
	Researcher Researcher
	Writer     Writer
}
type Result struct {
	NothingNew bool
	Reason     string
	Drafts     []domain.DraftCandidate
	Sources    map[string]domain.SourceEvidence
}

func (w Workflow) Run(ctx context.Context, assignment string, history []domain.StoryHistory, previouslySeen map[string]bool) (Result, error) {
	if w.Researcher == nil || w.Writer == nil {
		return Result{}, fmt.Errorf("research providers are not configured")
	}
	now := time.Now().UTC()
	windowStart := now.Add(-7 * 24 * time.Hour).Format("2006-01-02")
	query := fmt.Sprintf("%s recent developments from %s through %s UTC", assignment, windowStart, now.Format("2006-01-02"))
	sources, err := w.Researcher.Search(ctx, query, 5)
	if err != nil {
		return Result{}, fmt.Errorf("initial research: %w", err)
	}
	sources = assignIDs(sources, nil, previouslySeen)
	assessment, err := w.Writer.Assess(ctx, assignment, sources, history)
	if err != nil {
		return Result{}, fmt.Errorf("assess evidence: %w", err)
	}
	if assessment.Outcome == "nothing_new" {
		return Result{NothingNew: true, Reason: conciseReason(assessment.Reason)}, nil
	}
	if assessment.Outcome == "follow_up" {
		more, err := w.Researcher.Search(ctx, assessment.FollowUpQuery, 5)
		if err != nil {
			return Result{}, fmt.Errorf("follow-up research: %w", err)
		}
		sources = append(sources, assignIDs(more, sources, previouslySeen)...)
	} else if assessment.Outcome != "final" {
		return Result{}, fmt.Errorf("invalid assessment outcome %q", assessment.Outcome)
	}
	if len(sources) == 0 {
		return Result{NothingNew: true, Reason: "No source evidence was returned."}, nil
	}
	output, err := w.Writer.Draft(ctx, assignment, sources, history)
	if err != nil {
		return Result{}, fmt.Errorf("generate drafts: %w", err)
	}
	if output.Outcome == "nothing_new" {
		return Result{NothingNew: true, Reason: conciseReason(output.Reason)}, nil
	}
	if output.Outcome != "drafts" {
		return Result{}, fmt.Errorf("invalid draft outcome %q", output.Outcome)
	}
	if len(output.Drafts) == 0 {
		return Result{NothingNew: true, Reason: "No sufficiently supported new development was found."}, nil
	}
	if len(output.Drafts) > maxDrafts {
		return Result{}, fmt.Errorf("model returned too many drafts")
	}
	known := make(map[string]domain.SourceEvidence, len(sources))
	for _, source := range sources {
		known[source.ID] = source
	}
	for _, draft := range output.Drafts {
		if err := validateDraft(draft, known); err != nil {
			return Result{}, err
		}
	}
	return Result{Reason: conciseReason(output.Reason), Drafts: output.Drafts, Sources: known}, nil
}

func assignIDs(sources, existing []domain.SourceEvidence, previous map[string]bool) []domain.SourceEvidence {
	start := len(existing)
	seen := map[string]bool{}
	for _, source := range existing {
		seen[normalizeURL(source.URL)] = true
	}
	result := make([]domain.SourceEvidence, 0, len(sources))
	for _, source := range sources {
		key := normalizeURL(source.URL)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		source.ID = fmt.Sprintf("source_%d", start+len(result)+1)
		source.PreviouslySeen = previous[key]
		result = append(result, source)
		if len(result) == 5 {
			break
		}
	}
	return result
}

func validateDraft(draft domain.DraftCandidate, sources map[string]domain.SourceEvidence) error {
	if strings.TrimSpace(draft.Headline) == "" || strings.TrimSpace(draft.FacebookText) == "" || strings.TrimSpace(draft.XText) == "" {
		return fmt.Errorf("model returned an incomplete draft")
	}
	if len(draft.SourceIDs) == 0 {
		return fmt.Errorf("model returned a draft without source IDs")
	}
	if utf8.RuneCountInString(draft.XText) > 280 {
		return fmt.Errorf("model returned X text over the 280-character approximation")
	}
	if len(draft.FacebookText) > 5000 {
		return fmt.Errorf("model returned Facebook text over the allowed limit")
	}
	if draft.ClaimStatus != "official" && draft.ClaimStatus != "reported" && draft.ClaimStatus != "unverified" {
		return fmt.Errorf("invalid claim status %q", draft.ClaimStatus)
	}
	for _, id := range draft.SourceIDs {
		if _, ok := sources[id]; !ok {
			return fmt.Errorf("model referenced unknown source ID %q", id)
		}
	}
	if draft.ClaimStatus == "official" {
		if len(draft.OfficialSourceIDs) == 0 {
			return fmt.Errorf("official claim has no declared primary source")
		}
		for _, id := range draft.OfficialSourceIDs {
			if _, ok := sources[id]; !ok {
				return fmt.Errorf("model referenced unknown official source ID %q", id)
			}
			if !containsID(draft.SourceIDs, id) {
				return fmt.Errorf("official source ID %q is not listed in the draft evidence", id)
			}
		}
	}
	return nil
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func normalizeURL(raw string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "/")
}
func conciseReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "No sufficiently supported new development was found."
	}
	if len(reason) > 500 {
		return reason[:500]
	}
	return reason
}
