// Package domain contains provider-neutral newsroom workflow values.
package domain

import "time"

type SourceEvidence struct {
	ID             string
	URL            string
	Title          string
	ImageURL       string
	PublishedAt    *time.Time
	RetrievedAt    time.Time
	Content        string
	PreviouslySeen bool
}

type StoryHistory struct {
	Title   string
	Summary string
}

type Assessment struct {
	Outcome       string // final, follow_up, or nothing_new
	Reason        string
	FollowUpQuery string
}

type DraftCandidate struct {
	Headline          string
	ClaimStatus       string
	FacebookText      string
	XText             string
	SourceIDs         []string
	OfficialSourceIDs []string
}

type FinalOutput struct {
	Outcome string // drafts or nothing_new
	Reason  string
	Drafts  []DraftCandidate
}
