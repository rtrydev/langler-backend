package lessons

import (
	"context"
	"fmt"
	"sort"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

func (s *Service) Glossary(ctx context.Context, query inbound.GlossaryQuery) (inbound.GlossaryResult, error) {
	if query.Owner == "" {
		return inbound.GlossaryResult{}, domain.ErrInvalidOwner
	}
	languages := []domain.Language{domain.Language(query.Language)}
	if query.Language == "" {
		languages = domain.Languages()
	} else if !domain.KnownLanguage(domain.Language(query.Language)) {
		return inbound.GlossaryResult{}, &domain.ValidationError{Issues: []domain.Issue{{
			Path:    "language",
			Message: fmt.Sprintf("must be one of %s", joinStrings(domain.Languages())),
		}}}
	}

	result := inbound.GlossaryResult{Languages: make([]inbound.GlossaryLanguage, 0, len(languages))}
	for _, language := range languages {
		words, err := s.glossaryWords(ctx, query.Owner, language)
		if err != nil {
			return inbound.GlossaryResult{}, err
		}
		result.Languages = append(result.Languages, inbound.GlossaryLanguage{Language: string(language), Words: words})
	}
	return result, nil
}

func (s *Service) glossaryWords(ctx context.Context, owner string, language domain.Language) ([]inbound.GlossaryWord, error) {
	entries, err := s.glossary.Entries(ctx, owner, string(language), progress.KindVocab)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []inbound.GlossaryWord{}, nil
	}
	lang, err := reference.NewLanguage(string(language))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	vocab, err := s.reader.VocabByIDs(ctx, lang, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]reference.VocabEntry, len(vocab))
	for _, entry := range vocab {
		byID[entry.ID] = entry
	}

	words := make([]inbound.GlossaryWord, 0, len(entries))
	for _, entry := range entries {
		ref, ok := byID[entry.ID]
		if !ok {
			continue
		}
		words = append(words, inbound.GlossaryWord{
			ID:          entry.ID,
			Headword:    ref.Headword,
			Reading:     ref.Reading,
			Gloss:       ref.Gloss,
			Level:       string(ref.Level),
			LessonCount: entry.LessonCount,
			AddedAt:     entry.AddedAt,
		})
	}
	sort.SliceStable(words, func(i, j int) bool {
		if !words[i].AddedAt.Equal(words[j].AddedAt) {
			return words[i].AddedAt.After(words[j].AddedAt)
		}
		return words[i].ID < words[j].ID
	})
	return words, nil
}
