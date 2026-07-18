package lessons

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type Service struct {
	store   outbound.LessonStore
	checker outbound.ReferenceChecker
	reader  outbound.ReferenceReader
	now     func() time.Time
}

func NewService(store outbound.LessonStore, checker outbound.ReferenceChecker, reader outbound.ReferenceReader) (*Service, error) {
	if store == nil {
		return nil, errors.New("lesson store must not be nil")
	}
	if checker == nil {
		return nil, errors.New("reference checker must not be nil")
	}
	if reader == nil {
		return nil, errors.New("reference reader must not be nil")
	}
	return &Service{store: store, checker: checker, reader: reader, now: time.Now}, nil
}

func (s *Service) Import(ctx context.Context, command inbound.LessonImportCommand) (inbound.LessonImportResult, error) {
	if command.Owner == "" {
		return inbound.LessonImportResult{}, domain.ErrInvalidOwner
	}

	validated, err := domain.New(command.Lesson)
	if err != nil {
		return inbound.LessonImportResult{}, err
	}
	if err := s.checkReferences(ctx, validated); err != nil {
		return inbound.LessonImportResult{}, err
	}

	record := outbound.LessonRecord{
		Owner:       command.Owner,
		ContentHash: command.ContentHash,
		CreatedAt:   s.now().UTC(),
		Lesson:      validated,
	}
	saveErr := s.store.Save(ctx, record)
	if errors.Is(saveErr, domain.ErrAlreadyExists) {
		existing, err := s.store.Get(ctx, command.Owner, validated.ID)
		if err != nil {
			return inbound.LessonImportResult{}, err
		}
		return inbound.LessonImportResult{Stored: storedLesson(existing), Created: false}, nil
	}
	if saveErr != nil {
		return inbound.LessonImportResult{}, saveErr
	}
	return inbound.LessonImportResult{Stored: storedLesson(record), Created: true}, nil
}

func (s *Service) checkReferences(ctx context.Context, l domain.Lesson) error {
	lang, err := reference.NewLanguage(string(l.Language))
	if err != nil {
		return err
	}

	vocabIDs := collectIDs(l, func(e domain.Exercise) []string { return e.ReferencedVocab })
	grammarIDs := collectIDs(l, func(e domain.Exercise) []string { return e.ReferencedGrammar })

	missingVocab, err := s.missing(ctx, lang, vocabIDs, s.checker.MissingVocab)
	if err != nil {
		return err
	}
	missingGrammar, err := s.missing(ctx, lang, grammarIDs, s.checker.MissingGrammar)
	if err != nil {
		return err
	}
	if len(missingVocab) == 0 && len(missingGrammar) == 0 {
		return nil
	}

	var issues []domain.Issue
	for i, e := range l.Exercises {
		for j, id := range e.ReferencedVocab {
			if missingVocab[id] {
				issues = append(issues, domain.Issue{
					Path:    fmt.Sprintf("exercises[%d].referencedVocab[%d]", i, j),
					Message: fmt.Sprintf("vocabulary id %q does not exist in the %q reference data", id, lang),
				})
			}
		}
		for j, id := range e.ReferencedGrammar {
			if missingGrammar[id] {
				issues = append(issues, domain.Issue{
					Path:    fmt.Sprintf("exercises[%d].referencedGrammar[%d]", i, j),
					Message: fmt.Sprintf("grammar id %q does not exist in the %q reference data", id, lang),
				})
			}
		}
	}
	return &domain.ValidationError{Issues: issues}
}

func (s *Service) missing(
	ctx context.Context,
	lang reference.Language,
	ids []string,
	check func(context.Context, reference.Language, []string) ([]string, error),
) (map[string]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	missing, err := check(ctx, lang, ids)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(missing))
	for _, id := range missing {
		set[id] = true
	}
	return set, nil
}

func collectIDs(l domain.Lesson, pick func(domain.Exercise) []string) []string {
	seen := map[string]bool{}
	var ids []string
	for _, e := range l.Exercises {
		for _, id := range pick(e) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func (s *Service) List(ctx context.Context, query inbound.LessonListQuery) (inbound.LessonListResult, error) {
	if query.Owner == "" {
		return inbound.LessonListResult{}, domain.ErrInvalidOwner
	}
	page, err := s.store.List(ctx, query.Owner, clampLimit(query.Limit), query.Cursor)
	if err != nil {
		return inbound.LessonListResult{}, err
	}
	lessons := make([]inbound.StoredLesson, 0, len(page.Records))
	for _, record := range page.Records {
		lessons = append(lessons, storedLesson(record))
	}
	return inbound.LessonListResult{Lessons: lessons, NextCursor: page.NextCursor}, nil
}

func (s *Service) Get(ctx context.Context, query inbound.LessonQuery) (inbound.StoredLesson, error) {
	if query.Owner == "" {
		return inbound.StoredLesson{}, domain.ErrInvalidOwner
	}
	if !domain.ValidID(query.ID) {
		return inbound.StoredLesson{}, domain.ErrInvalidLessonID
	}
	record, err := s.store.Get(ctx, query.Owner, query.ID)
	if err != nil {
		return inbound.StoredLesson{}, err
	}
	return storedLesson(record), nil
}

func (s *Service) Delete(ctx context.Context, query inbound.LessonQuery) error {
	if query.Owner == "" {
		return domain.ErrInvalidOwner
	}
	if !domain.ValidID(query.ID) {
		return domain.ErrInvalidLessonID
	}
	return s.store.Delete(ctx, query.Owner, query.ID)
}

func storedLesson(record outbound.LessonRecord) inbound.StoredLesson {
	return inbound.StoredLesson{Lesson: record.Lesson, CreatedAt: record.CreatedAt}
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	return min(limit, maxLimit)
}
