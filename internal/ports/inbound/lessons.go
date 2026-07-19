package inbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
)

type LessonImportCommand struct {
	Owner          string
	ContentHash    string
	IdempotencyKey string
	Lesson         lesson.Lesson
}

type StoredLesson struct {
	Lesson    lesson.Lesson
	CreatedAt time.Time
}

type LessonImportResult struct {
	Stored  StoredLesson
	Created bool
}

type LessonImporter interface {
	Import(ctx context.Context, command LessonImportCommand) (LessonImportResult, error)
}

type LessonListQuery struct {
	Owner  string
	Limit  int
	Cursor string
}

type LessonListResult struct {
	Lessons    []StoredLesson
	NextCursor string
}

type LessonQuery struct {
	Owner string
	ID    string
}

type LessonLibrary interface {
	List(ctx context.Context, query LessonListQuery) (LessonListResult, error)
	Get(ctx context.Context, query LessonQuery) (StoredLesson, error)
	Delete(ctx context.Context, query LessonQuery) error
}

type LessonPromptQuery struct {
	Owner            string
	Language         string
	Level            string
	Topic            string
	TopicSlug        string
	ExerciseTypes    []string
	ReadingStage     string
	Length           string
	IncludeReference bool
}

type LessonPrompt struct {
	Prompt string
}

type LessonPromptBuilder interface {
	Build(ctx context.Context, query LessonPromptQuery) (LessonPrompt, error)
}

type LessonTopicsQuery struct {
	Owner    string
	Language string
	Level    string
}

type LessonTopic struct {
	Slug         string
	Name         string
	Description  string
	WordCount    int
	CoveredCount int
}

type LessonTopicsResult struct {
	Topics []LessonTopic
}

type LessonTopicAdvisor interface {
	Topics(ctx context.Context, query LessonTopicsQuery) (LessonTopicsResult, error)
}

type LessonResultCommand struct {
	Owner       string
	CompletedOn time.Time
	Result      lesson.Result
}

type LessonResultRecorder interface {
	Record(ctx context.Context, command LessonResultCommand) (lesson.Result, error)
}
