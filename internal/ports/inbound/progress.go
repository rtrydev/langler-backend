package inbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
)

type LessonProgressRecorder interface {
	RecordLesson(ctx context.Context, owner string, source lesson.Lesson, result lesson.Result) error
}

type DueReviewQuery struct {
	Owner    string
	Language string
	DueOn    time.Time
}

type DueLanguage struct {
	Language string
	Items    []progress.Item
}

type DueReviews struct {
	Languages []DueLanguage
}

type ReviewGradeCommand struct {
	Owner      string
	Language   string
	Kind       progress.ItemKind
	ItemID     string
	Grade      progress.Grade
	ReviewedAt time.Time
}

type ReviewQueue interface {
	Due(ctx context.Context, query DueReviewQuery) (DueReviews, error)
	Grade(ctx context.Context, command ReviewGradeCommand) (progress.Item, error)
}

type RecentLesson struct {
	LessonID    string
	Title       string
	Score       int
	MaxScore    int
	CompletedAt time.Time
}

type ReviewDay struct {
	Date  string
	Count int
}

type LanguageProgress struct {
	Language            string
	LessonsCompleted    int
	ItemsTracked        int
	DueToday            int
	CurrentReviewStreak int
	ReviewHistory       []ReviewDay
	RecentLessons       []RecentLesson
}

type ProgressSummaryQuery struct {
	Owner string
	DueOn time.Time
}

type ProgressSummary struct {
	Languages []LanguageProgress
}

type ProgressReporter interface {
	Summary(ctx context.Context, query ProgressSummaryQuery) (ProgressSummary, error)
}

type ProgressProvider interface {
	ReviewQueue
	ProgressReporter
}
