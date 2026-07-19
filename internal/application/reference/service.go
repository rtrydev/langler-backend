package reference

import (
	"context"
	"errors"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type Service struct {
	reader   outbound.ReferenceReader
	readings outbound.ReadingReader
}

func NewService(reader outbound.ReferenceReader) (*Service, error) {
	if reader == nil {
		return nil, errors.New("reference reader must not be nil")
	}
	readings, ok := reader.(outbound.ReadingReader)
	if !ok {
		return nil, errors.New("reference reader must support readings")
	}
	return &Service{reader: reader, readings: readings}, nil
}

func (s *Service) Vocab(ctx context.Context, query inbound.VocabQuery) (inbound.VocabResult, error) {
	lang, err := domain.NewLanguage(query.Language)
	if err != nil {
		return inbound.VocabResult{}, err
	}
	level, err := optionalLevel(query.Level)
	if err != nil {
		return inbound.VocabResult{}, err
	}
	topic, err := optionalTopic(query.Topic)
	if err != nil {
		return inbound.VocabResult{}, err
	}

	page, err := s.reader.Vocab(ctx, outbound.VocabFilter{
		Language: lang,
		Level:    level,
		Topic:    topic,
		Limit:    clampLimit(query.Limit),
		Cursor:   query.Cursor,
	})
	if err != nil {
		return inbound.VocabResult{}, err
	}
	return inbound.VocabResult{Entries: page.Entries, NextCursor: page.NextCursor}, nil
}

func (s *Service) Grammar(ctx context.Context, query inbound.GrammarQuery) (inbound.GrammarResult, error) {
	lang, err := domain.NewLanguage(query.Language)
	if err != nil {
		return inbound.GrammarResult{}, err
	}
	level, err := optionalLevel(query.Level)
	if err != nil {
		return inbound.GrammarResult{}, err
	}

	page, err := s.reader.Grammar(ctx, outbound.GrammarFilter{
		Language: lang,
		Level:    level,
		Limit:    clampLimit(query.Limit),
		Cursor:   query.Cursor,
	})
	if err != nil {
		return inbound.GrammarResult{}, err
	}
	return inbound.GrammarResult{Topics: page.Topics, NextCursor: page.NextCursor}, nil
}

func (s *Service) Scripts(ctx context.Context, query inbound.ScriptQuery) (inbound.ScriptResult, error) {
	lang, err := domain.NewLanguage(query.Language)
	if err != nil {
		return inbound.ScriptResult{}, err
	}
	scriptType, err := optionalScriptType(query.ScriptType)
	if err != nil {
		return inbound.ScriptResult{}, err
	}
	level, err := optionalLevel(query.Level)
	if err != nil {
		return inbound.ScriptResult{}, err
	}
	if level != "" && scriptType == "" {
		return inbound.ScriptResult{}, domain.ErrLevelWithoutType
	}

	page, err := s.reader.Scripts(ctx, outbound.ScriptFilter{
		Language:   lang,
		ScriptType: scriptType,
		Level:      level,
		Limit:      clampLimit(query.Limit),
		Cursor:     query.Cursor,
	})
	if err != nil {
		return inbound.ScriptResult{}, err
	}
	return inbound.ScriptResult{Glyphs: page.Glyphs, NextCursor: page.NextCursor}, nil
}

func (s *Service) Readings(ctx context.Context, query inbound.ReadingQuery) (inbound.ReadingResult, error) {
	lang, err := domain.NewLanguage(query.Language)
	if err != nil {
		return inbound.ReadingResult{}, err
	}
	level, err := optionalLevel(query.Level)
	if err != nil {
		return inbound.ReadingResult{}, err
	}
	page, err := s.readings.Readings(ctx, outbound.ReadingFilter{
		Language: lang, Level: level, Limit: clampLimit(query.Limit), Cursor: query.Cursor,
	})
	if err != nil {
		return inbound.ReadingResult{}, err
	}
	return inbound.ReadingResult{Passages: page.Passages, NextCursor: page.NextCursor}, nil
}

func optionalLevel(band string) (domain.Level, error) {
	if band == "" {
		return "", nil
	}
	return domain.NewLevel(band)
}

func optionalTopic(tag string) (domain.TopicTag, error) {
	if tag == "" {
		return "", nil
	}
	return domain.NewTopicTag(tag)
}

func optionalScriptType(name string) (domain.ScriptType, error) {
	if name == "" {
		return "", nil
	}
	return domain.NewScriptType(name)
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultLimit
	case limit > maxLimit:
		return maxLimit
	default:
		return limit
	}
}
