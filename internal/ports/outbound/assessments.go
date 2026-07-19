package outbound

import (
	"context"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/assessment"
)

type ProfileLevelRecord struct {
	Language     string
	Level        string
	AssessmentID string
	UpdatedAt    time.Time
}

type AssessmentStore interface {
	Create(ctx context.Context, owner string, session assessment.Session) error
	Get(ctx context.Context, owner, id string) (assessment.Session, error)
	Save(ctx context.Context, owner string, session assessment.Session, level *ProfileLevelRecord) error
	List(ctx context.Context, owner string) ([]assessment.Session, error)
	Levels(ctx context.Context, owner string) ([]ProfileLevelRecord, error)
}
