package outbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/progress"
)

type GlossaryRefs struct {
	VocabIDs   []string
	GrammarIDs []string
}

func (r GlossaryRefs) Empty() bool {
	return len(r.VocabIDs) == 0 && len(r.GrammarIDs) == 0
}

type GlossaryEntry struct {
	ID          string
	Language    string
	Kind        progress.ItemKind
	LessonCount int
	AddedAt     time.Time
}

type GlossaryStore interface {
	AddLessonWords(ctx context.Context, owner, language, lessonID string, refs GlossaryRefs, addedAt time.Time) error
	RemoveLessonWords(ctx context.Context, owner, language, lessonID string, refs GlossaryRefs) error
	Entries(ctx context.Context, owner, language string, kind progress.ItemKind) ([]GlossaryEntry, error)
	GlossaryItemIDs(ctx context.Context, owner, language string, kind progress.ItemKind) ([]string, error)
}
