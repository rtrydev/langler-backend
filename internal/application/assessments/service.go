package assessments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"slices"
	"time"

	domain "github.com/rtrydev/langler-backend/internal/domain/assessment"
	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

const (
	poolPageSize = 200
	maxPoolPages = 25
)

type Service struct {
	store  outbound.AssessmentStore
	reader outbound.ReferenceReader
	now    func() time.Time
	intn   func(int) int
	random func([]byte) (int, error)
}

func NewService(store outbound.AssessmentStore, reader outbound.ReferenceReader) (*Service, error) {
	if store == nil {
		return nil, errors.New("assessment store must not be nil")
	}
	if reader == nil {
		return nil, errors.New("reference reader must not be nil")
	}
	return &Service{store: store, reader: reader, now: time.Now, intn: mathrand.IntN, random: rand.Read}, nil
}

func (s *Service) Start(ctx context.Context, command inbound.AssessmentStartCommand) (inbound.AssessmentView, error) {
	if command.Owner == "" {
		return inbound.AssessmentView{}, lesson.ErrInvalidOwner
	}
	id, err := s.newID()
	if err != nil {
		return inbound.AssessmentView{}, err
	}
	session, err := domain.NewSession(id, command.Language, s.now())
	if err != nil {
		return inbound.AssessmentView{}, err
	}
	stage, err := s.buildStage(ctx, session.Language, session.Bands[0])
	if err != nil {
		return inbound.AssessmentView{}, err
	}
	session.Stages = []domain.Stage{stage}
	if err := s.store.Create(ctx, command.Owner, session); err != nil {
		return inbound.AssessmentView{}, err
	}
	return toView(session), nil
}

func (s *Service) Answer(ctx context.Context, command inbound.AssessmentAnswerCommand) (inbound.AssessmentView, error) {
	if command.Owner == "" {
		return inbound.AssessmentView{}, lesson.ErrInvalidOwner
	}
	if command.AssessmentID == "" {
		return inbound.AssessmentView{}, domain.ErrNotFound
	}
	session, err := s.store.Get(ctx, command.Owner, command.AssessmentID)
	if err != nil {
		return inbound.AssessmentView{}, err
	}
	if view, replayed := replayView(session, command); replayed {
		return view, nil
	}
	updated, err := domain.Answer(session, command.StageIndex, command.Answers, s.now())
	if err != nil {
		return inbound.AssessmentView{}, err
	}
	var level *outbound.ProfileLevelRecord
	if updated.Status == domain.StatusCompleted {
		level = &outbound.ProfileLevelRecord{
			Language:     updated.Language,
			Level:        updated.EstimatedLevel,
			AssessmentID: updated.ID,
			UpdatedAt:    updated.CompletedAt,
		}
	} else if band, ok := domain.NextBand(updated); ok {
		stage, err := s.buildStage(ctx, updated.Language, band)
		if err != nil {
			return inbound.AssessmentView{}, err
		}
		updated.Stages = append(updated.Stages, stage)
	}
	if err := s.store.Save(ctx, command.Owner, updated, level); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			stored, getErr := s.store.Get(ctx, command.Owner, command.AssessmentID)
			if getErr == nil {
				if view, replayed := replayView(stored, command); replayed {
					return view, nil
				}
			}
		}
		return inbound.AssessmentView{}, err
	}
	return toView(updated), nil
}

func replayView(session domain.Session, command inbound.AssessmentAnswerCommand) (inbound.AssessmentView, bool) {
	if command.StageIndex < 0 || command.StageIndex >= len(session.Stages) {
		return inbound.AssessmentView{}, false
	}
	stage := session.Stages[command.StageIndex]
	if !stage.Answered || !slices.Equal(stage.Answers, command.Answers) {
		return inbound.AssessmentView{}, false
	}
	return toView(session), true
}

func (s *Service) Assessment(ctx context.Context, owner, id string) (inbound.AssessmentView, error) {
	if owner == "" {
		return inbound.AssessmentView{}, lesson.ErrInvalidOwner
	}
	if id == "" {
		return inbound.AssessmentView{}, domain.ErrNotFound
	}
	session, err := s.store.Get(ctx, owner, id)
	if err != nil {
		return inbound.AssessmentView{}, err
	}
	return toView(session), nil
}

func (s *Service) Assessments(ctx context.Context, owner string) ([]inbound.AssessmentSummary, error) {
	if owner == "" {
		return nil, lesson.ErrInvalidOwner
	}
	sessions, err := s.store.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(sessions, func(a, b domain.Session) int {
		return b.StartedAt.Compare(a.StartedAt)
	})
	summaries := make([]inbound.AssessmentSummary, 0, len(sessions))
	for _, session := range sessions {
		summaries = append(summaries, inbound.AssessmentSummary{
			ID:             session.ID,
			Language:       session.Language,
			Status:         string(session.Status),
			EstimatedLevel: session.EstimatedLevel,
			Confidence:     string(session.Confidence),
			Floor:          session.Floor,
			StartedAt:      session.StartedAt,
			CompletedAt:    session.CompletedAt,
		})
	}
	return summaries, nil
}

func (s *Service) Levels(ctx context.Context, owner string) ([]inbound.ProfileLevel, error) {
	if owner == "" {
		return nil, lesson.ErrInvalidOwner
	}
	records, err := s.store.Levels(ctx, owner)
	if err != nil {
		return nil, err
	}
	levels := make([]inbound.ProfileLevel, 0, len(records))
	for _, record := range records {
		levels = append(levels, inbound.ProfileLevel{
			Language:     record.Language,
			Level:        record.Level,
			AssessmentID: record.AssessmentID,
			UpdatedAt:    record.UpdatedAt,
		})
	}
	return levels, nil
}

func (s *Service) newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := s.random(raw); err != nil {
		return "", fmt.Errorf("generate assessment id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (s *Service) buildStage(ctx context.Context, language, band string) (domain.Stage, error) {
	lang, err := reference.NewLanguage(language)
	if err != nil {
		return domain.Stage{}, err
	}
	level, err := reference.NewLevel(band)
	if err != nil {
		return domain.Stage{}, err
	}
	vocab, err := s.vocabPool(ctx, lang, level)
	if err != nil {
		return domain.Stage{}, err
	}
	grammar, err := s.grammarPool(ctx, lang, level)
	if err != nil {
		return domain.Stage{}, err
	}
	return domain.BuildStage(band, vocab, grammar, s.intn)
}

func (s *Service) vocabPool(ctx context.Context, lang reference.Language, level reference.Level) ([]domain.VocabCandidate, error) {
	var pool []domain.VocabCandidate
	cursor := ""
	for range maxPoolPages {
		page, err := s.reader.Vocab(ctx, outbound.VocabFilter{Language: lang, Level: level, Limit: poolPageSize, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, entry := range page.Entries {
			candidate := domain.VocabCandidate{
				ID:            entry.ID,
				Headword:      entry.Headword,
				Reading:       entry.Reading,
				PartsOfSpeech: entry.PartsOfSpeech,
			}
			if len(entry.Gloss) > 0 {
				candidate.Gloss = entry.Gloss[0]
			}
			if entry.Example != nil {
				candidate.Example = entry.Example.Text
				candidate.ExampleTranslation = entry.Example.Translation
			}
			pool = append(pool, candidate)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	return pool, nil
}

func (s *Service) grammarPool(ctx context.Context, lang reference.Language, level reference.Level) ([]domain.GrammarCandidate, error) {
	var pool []domain.GrammarCandidate
	cursor := ""
	for range maxPoolPages {
		page, err := s.reader.Grammar(ctx, outbound.GrammarFilter{Language: lang, Level: level, Limit: poolPageSize, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		for _, topic := range page.Topics {
			candidate := domain.GrammarCandidate{ID: topic.ID}
			if topic.Example != nil {
				candidate.Example = topic.Example.Text
				candidate.ExampleTranslation = topic.Example.Translation
			}
			pool = append(pool, candidate)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	return pool, nil
}

func toView(session domain.Session) inbound.AssessmentView {
	view := inbound.AssessmentView{
		ID:          session.ID,
		Language:    session.Language,
		Status:      string(session.Status),
		StartedAt:   session.StartedAt,
		CompletedAt: session.CompletedAt,
	}
	if session.Status == domain.StatusInProgress && len(session.Stages) > 0 {
		index := len(session.Stages) - 1
		stage := session.Stages[index]
		items := make([]inbound.AssessmentItemView, 0, len(stage.Items))
		for _, item := range stage.Items {
			items = append(items, inbound.AssessmentItemView{
				Kind:    string(item.Kind),
				Prompt:  item.Prompt,
				Options: append([]string(nil), item.Options...),
			})
		}
		view.Stage = &inbound.AssessmentStageView{
			Index:     index,
			Band:      stage.Band,
			BandCount: len(session.Bands),
			Items:     items,
		}
	}
	if session.Status == domain.StatusCompleted {
		bands := make([]inbound.AssessmentBandResult, 0, len(session.Stages))
		for _, stage := range session.Stages {
			bands = append(bands, inbound.AssessmentBandResult{
				Band:    stage.Band,
				Correct: stage.Correct,
				Total:   len(stage.Items),
				Passed:  domain.Passed(stage),
			})
		}
		view.Result = &inbound.AssessmentResultView{
			EstimatedLevel: session.EstimatedLevel,
			Confidence:     string(session.Confidence),
			Floor:          session.Floor,
			Bands:          bands,
		}
	}
	return view
}
