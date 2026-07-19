package inbound

import (
	"context"
	"time"
)

type AssessmentStartCommand struct {
	Owner    string
	Language string
}

type AssessmentAnswerCommand struct {
	Owner        string
	AssessmentID string
	StageIndex   int
	Answers      []int
}

type AssessmentItemView struct {
	Kind    string
	Prompt  string
	Options []string
}

type AssessmentStageView struct {
	Index     int
	Band      string
	BandCount int
	Items     []AssessmentItemView
}

type AssessmentBandResult struct {
	Band    string
	Correct int
	Total   int
	Passed  bool
}

type AssessmentResultView struct {
	EstimatedLevel string
	Confidence     string
	Floor          bool
	Bands          []AssessmentBandResult
}

type AssessmentView struct {
	ID          string
	Language    string
	Status      string
	StartedAt   time.Time
	CompletedAt time.Time
	Stage       *AssessmentStageView
	Result      *AssessmentResultView
}

type AssessmentSummary struct {
	ID             string
	Language       string
	Status         string
	EstimatedLevel string
	Confidence     string
	Floor          bool
	StartedAt      time.Time
	CompletedAt    time.Time
}

type ProfileLevel struct {
	Language     string
	Level        string
	AssessmentID string
	UpdatedAt    time.Time
}

type AssessmentProvider interface {
	Start(ctx context.Context, command AssessmentStartCommand) (AssessmentView, error)
	Answer(ctx context.Context, command AssessmentAnswerCommand) (AssessmentView, error)
	Assessment(ctx context.Context, owner, id string) (AssessmentView, error)
	Assessments(ctx context.Context, owner string) ([]AssessmentSummary, error)
	Levels(ctx context.Context, owner string) ([]ProfileLevel, error)
}
