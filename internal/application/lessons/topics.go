package lessons

import (
	"context"
	"fmt"
	"sort"
	"strings"

	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

func (s *Service) Topics(ctx context.Context, query inbound.LessonTopicsQuery) (inbound.LessonTopicsResult, error) {
	if query.Owner == "" {
		return inbound.LessonTopicsResult{}, domain.ErrInvalidOwner
	}

	language := domain.Language(query.Language)
	level := domain.Level(strings.ToUpper(query.Level))
	var issues []domain.Issue
	if !domain.KnownLanguage(language) {
		issues = append(issues, domain.Issue{Path: "language", Message: fmt.Sprintf("must be one of %s", joinStrings(domain.Languages()))})
	} else if !domain.KnownLevel(language, level) {
		issues = append(issues, domain.Issue{Path: "level", Message: fmt.Sprintf("must be one of %s for language %q", joinStrings(domain.LevelsFor(language)), language)})
	}
	if len(issues) > 0 {
		return inbound.LessonTopicsResult{}, &domain.ValidationError{Issues: issues}
	}

	lang, err := reference.NewLanguage(string(language))
	if err != nil {
		return inbound.LessonTopicsResult{}, err
	}
	refLevel, err := reference.NewLevel(string(level))
	if err != nil {
		return inbound.LessonTopicsResult{}, err
	}

	topics, err := s.reader.Topics(ctx, outbound.TopicFilter{Language: lang, Level: refLevel})
	if err != nil {
		return inbound.LessonTopicsResult{}, err
	}
	covered, err := s.coveredIDs(ctx, query.Owner, string(language), progress.KindVocab)
	if err != nil {
		return inbound.LessonTopicsResult{}, err
	}

	result := make([]inbound.LessonTopic, 0, len(topics))
	for _, topic := range topics {
		coveredCount := 0
		for _, id := range topic.VocabIDs {
			if covered[id] {
				coveredCount++
			}
		}
		result = append(result, inbound.LessonTopic{
			Slug:         string(topic.Slug),
			Name:         topic.Name,
			Description:  topic.Description,
			WordCount:    len(topic.VocabIDs),
			CoveredCount: coveredCount,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := result[i], result[j]
		leftRatio := left.CoveredCount * max(right.WordCount, 1)
		rightRatio := right.CoveredCount * max(left.WordCount, 1)
		if leftRatio != rightRatio {
			return leftRatio < rightRatio
		}
		return left.Slug < right.Slug
	})
	return inbound.LessonTopicsResult{Topics: result}, nil
}
