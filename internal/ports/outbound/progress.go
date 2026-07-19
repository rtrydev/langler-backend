package outbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
)

type LessonProgressRecorder interface {
	RecordLesson(ctx context.Context, owner string, source lesson.Lesson, result lesson.Result, completedOn time.Time) error
}

type ReferenceContext struct {
	ID             string
	Kind           progress.ItemKind
	Headword       string
	Reading        string
	Gloss          string
	Example        string
	ExampleMeaning string
}

type ProgressReferenceLookup interface {
	LookupProgress(ctx context.Context, language string, vocabIDs, grammarIDs []string) (map[string]ReferenceContext, error)
}

type ProgressSnapshot struct {
	Items          []progress.Item
	LessonActivity []progress.LessonActivity
	ReviewActivity []progress.ReviewActivity
}

type CoverageReader interface {
	CoveredItemIDs(ctx context.Context, owner, language string, kind progress.ItemKind) ([]string, error)
}

type ProgressStore interface {
	GetItems(ctx context.Context, owner, language string, keys []string) (map[string]progress.Item, error)
	SaveLesson(ctx context.Context, owner string, items []progress.Item, activity progress.LessonActivity) error
	DueItems(ctx context.Context, owner, language string, dueOn time.Time) ([]progress.Item, error)
	SaveReview(ctx context.Context, owner string, item progress.Item, activity progress.ReviewActivity) error
	Snapshot(ctx context.Context, owner string) (ProgressSnapshot, error)
}
