package outbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
)

type LessonRecord struct {
	Owner       string
	ContentHash string
	CreatedAt   time.Time
	Lesson      lesson.Lesson
}

type LessonPage struct {
	Records    []LessonRecord
	NextCursor string
}

type LessonStore interface {
	Save(ctx context.Context, record LessonRecord) error
	Get(ctx context.Context, owner, id string) (LessonRecord, error)
	List(ctx context.Context, owner string, limit int, cursor string) (LessonPage, error)
	Delete(ctx context.Context, owner, id string) error
}

type ReferenceChecker interface {
	MissingVocab(ctx context.Context, language reference.Language, ids []string) ([]string, error)
	MissingGrammar(ctx context.Context, language reference.Language, ids []string) ([]string, error)
}
