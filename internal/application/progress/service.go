package progress

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	domain "github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type Service struct {
	store      outbound.ProgressStore
	references outbound.ProgressReferenceLookup
	now        func() time.Time
}

func NewService(store outbound.ProgressStore, references outbound.ProgressReferenceLookup) (*Service, error) {
	if store == nil {
		return nil, errors.New("progress store must not be nil")
	}
	if references == nil {
		return nil, errors.New("progress reference lookup must not be nil")
	}
	return &Service{store: store, references: references, now: time.Now}, nil
}

type performance struct {
	kind    domain.ItemKind
	id      string
	correct int
	total   int
}

func (s *Service) RecordLesson(ctx context.Context, owner string, source lesson.Lesson, result lesson.Result) error {
	if owner == "" {
		return lesson.ErrInvalidOwner
	}
	performances := lessonPerformances(source, result)
	vocabIDs, grammarIDs, keys := splitReferences(performances)
	contexts, err := s.references.LookupProgress(ctx, string(source.Language), vocabIDs, grammarIDs)
	if err != nil {
		return err
	}
	existing, err := s.store.GetItems(ctx, owner, string(source.Language), keys)
	if err != nil {
		return err
	}

	items := make([]domain.Item, 0, len(performances))
	completedAt := resultTime(result, s.now())
	for _, outcome := range performances {
		key := itemKey(outcome.kind, outcome.id)
		item, ok := existing[key]
		if !ok {
			context, found := contexts[key]
			if !found {
				return fmt.Errorf("%w: reference context for %s", domain.ErrInvalidItem, outcome.id)
			}
			item, err = domain.NewItem(domain.Item{
				ID: outcome.id, Language: string(source.Language), Kind: outcome.kind,
				Headword: context.Headword, Reading: context.Reading, Gloss: context.Gloss,
				Example: context.Example, ExampleMeaning: context.ExampleMeaning,
			}, completedAt)
			if err != nil {
				return err
			}
		}
		if item.LastLessonAttemptID == result.AttemptID {
			items = append(items, item)
			continue
		}
		item, err = domain.Schedule(item, domain.GradePerformance(outcome.correct, outcome.total), completedAt)
		if err != nil {
			return err
		}
		item.LastLessonAttemptID = result.AttemptID
		items = append(items, item)
	}

	return s.store.SaveLesson(ctx, owner, items, domain.LessonActivity{
		AttemptID: result.AttemptID, LessonID: source.ID, Language: string(source.Language),
		Title: source.Title, Score: result.Score, MaxScore: result.MaxScore, CompletedAt: result.CompletedAt,
	})
}

func resultTime(result lesson.Result, fallback time.Time) time.Time {
	if result.CompletedAt.IsZero() {
		return fallback.UTC()
	}
	return result.CompletedAt.UTC()
}

func lessonPerformances(source lesson.Lesson, result lesson.Result) []performance {
	outcomes := make(map[string]lesson.ExerciseResult, len(result.Exercises))
	for _, outcome := range result.Exercises {
		outcomes[outcome.ExerciseID] = outcome
	}
	byKey := map[string]*performance{}
	order := make([]string, 0)
	for _, exercise := range source.Exercises {
		outcome, ok := outcomes[exercise.ID]
		if !ok {
			continue
		}
		for _, reference := range exercise.ReferencedVocab {
			addPerformance(byKey, &order, domain.KindVocab, reference, outcome)
		}
		for _, reference := range exercise.ReferencedGrammar {
			addPerformance(byKey, &order, domain.KindGrammar, reference, outcome)
		}
	}
	items := make([]performance, 0, len(order))
	for _, key := range order {
		items = append(items, *byKey[key])
	}
	return items
}

func addPerformance(byKey map[string]*performance, order *[]string, kind domain.ItemKind, id string, outcome lesson.ExerciseResult) {
	key := itemKey(kind, id)
	item, ok := byKey[key]
	if !ok {
		item = &performance{kind: kind, id: id}
		byKey[key] = item
		*order = append(*order, key)
	}
	item.correct += outcome.Correct
	item.total += outcome.Total
}

func splitReferences(items []performance) ([]string, []string, []string) {
	var vocab, grammar, keys []string
	for _, item := range items {
		keys = append(keys, itemKey(item.kind, item.id))
		if item.kind == domain.KindVocab {
			vocab = append(vocab, item.id)
		} else {
			grammar = append(grammar, item.id)
		}
	}
	return vocab, grammar, keys
}

func itemKey(kind domain.ItemKind, id string) string {
	return string(kind) + "#" + id
}

func (s *Service) Due(ctx context.Context, query inbound.DueReviewQuery) (inbound.DueReviews, error) {
	if query.Owner == "" {
		return inbound.DueReviews{}, lesson.ErrInvalidOwner
	}
	languages := []lesson.Language{lesson.Language(query.Language)}
	if query.Language == "" {
		languages = lesson.Languages()
	} else if !lesson.KnownLanguage(lesson.Language(query.Language)) {
		return inbound.DueReviews{}, domain.ErrInvalidItem
	}
	dueOn := query.DueOn
	if dueOn.IsZero() {
		dueOn = s.now().UTC()
	}
	result := inbound.DueReviews{Languages: make([]inbound.DueLanguage, 0, len(languages))}
	for _, language := range languages {
		items, err := s.store.DueItems(ctx, query.Owner, string(language), dueOn)
		if err != nil {
			return inbound.DueReviews{}, err
		}
		result.Languages = append(result.Languages, inbound.DueLanguage{Language: string(language), Items: items})
	}
	return result, nil
}

func (s *Service) Grade(ctx context.Context, command inbound.ReviewGradeCommand) (domain.Item, error) {
	if command.Owner == "" {
		return domain.Item{}, lesson.ErrInvalidOwner
	}
	if !lesson.KnownLanguage(lesson.Language(command.Language)) || !domain.KnownKind(command.Kind) || command.ItemID == "" || !domain.KnownGrade(command.Grade) {
		return domain.Item{}, domain.ErrInvalidItem
	}
	items, err := s.store.GetItems(ctx, command.Owner, command.Language, []string{itemKey(command.Kind, command.ItemID)})
	if err != nil {
		return domain.Item{}, err
	}
	item, ok := items[itemKey(command.Kind, command.ItemID)]
	if !ok {
		return domain.Item{}, domain.ErrNotFound
	}
	reviewedAt := command.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = s.now().UTC()
	}
	item, err = domain.Schedule(item, command.Grade, reviewedAt)
	if err != nil {
		return domain.Item{}, err
	}
	err = s.store.SaveReview(ctx, command.Owner, item, domain.ReviewActivity{
		ItemID: command.ItemID, Language: command.Language, Grade: command.Grade, ReviewedAt: reviewedAt,
	})
	return item, err
}

func (s *Service) Summary(ctx context.Context, query inbound.ProgressSummaryQuery) (inbound.ProgressSummary, error) {
	if query.Owner == "" {
		return inbound.ProgressSummary{}, lesson.ErrInvalidOwner
	}
	snapshot, err := s.store.Snapshot(ctx, query.Owner)
	if err != nil {
		return inbound.ProgressSummary{}, err
	}
	dueOn := query.DueOn
	if dueOn.IsZero() {
		dueOn = s.now().UTC()
	}
	return summarize(snapshot, dueOn), nil
}

func summarize(snapshot outbound.ProgressSnapshot, dueOn time.Time) inbound.ProgressSummary {
	byLanguage := make(map[string]*inbound.LanguageProgress)
	for _, language := range lesson.Languages() {
		code := string(language)
		byLanguage[code] = &inbound.LanguageProgress{Language: code}
	}
	for _, item := range snapshot.Items {
		stats := languageStats(byLanguage, item.Language)
		stats.ItemsTracked++
		if !item.DueDate.After(endOfDay(dueOn)) {
			stats.DueToday++
		}
	}

	lessonIDs := map[string]map[string]bool{}
	for _, activity := range snapshot.LessonActivity {
		stats := languageStats(byLanguage, activity.Language)
		if lessonIDs[activity.Language] == nil {
			lessonIDs[activity.Language] = map[string]bool{}
		}
		lessonIDs[activity.Language][activity.LessonID] = true
		stats.RecentLessons = append(stats.RecentLessons, inbound.RecentLesson{
			LessonID: activity.LessonID, Title: activity.Title, Score: activity.Score,
			MaxScore: activity.MaxScore, CompletedAt: activity.CompletedAt,
		})
	}
	for language, ids := range lessonIDs {
		byLanguage[language].LessonsCompleted = len(ids)
	}

	reviewDays := map[string]map[string]int{}
	for _, activity := range snapshot.ReviewActivity {
		if reviewDays[activity.Language] == nil {
			reviewDays[activity.Language] = map[string]int{}
		}
		reviewDays[activity.Language][dateKey(activity.ReviewedAt)]++
	}

	languages := make([]inbound.LanguageProgress, 0, len(byLanguage))
	for _, language := range lesson.Languages() {
		stats := byLanguage[string(language)]
		sort.Slice(stats.RecentLessons, func(i, j int) bool {
			return stats.RecentLessons[i].CompletedAt.After(stats.RecentLessons[j].CompletedAt)
		})
		if len(stats.RecentLessons) > 3 {
			stats.RecentLessons = stats.RecentLessons[:3]
		}
		stats.ReviewHistory = history(reviewDays[string(language)], dueOn)
		stats.CurrentReviewStreak = streak(reviewDays[string(language)], dueOn)
		languages = append(languages, *stats)
	}
	return inbound.ProgressSummary{Languages: languages}
}

func languageStats(all map[string]*inbound.LanguageProgress, language string) *inbound.LanguageProgress {
	if all[language] == nil {
		all[language] = &inbound.LanguageProgress{Language: language}
	}
	return all[language]
}

func history(days map[string]int, dueOn time.Time) []inbound.ReviewDay {
	if len(days) == 0 {
		return []inbound.ReviewDay{}
	}
	cutoff := startOfDay(dueOn).AddDate(0, 0, -29)
	result := make([]inbound.ReviewDay, 0, len(days))
	for date, count := range days {
		parsed, err := time.Parse(time.DateOnly, date)
		if err == nil && !parsed.Before(cutoff) {
			result = append(result, inbound.ReviewDay{Date: date, Count: count})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date < result[j].Date })
	return result
}

func streak(days map[string]int, dueOn time.Time) int {
	count := 0
	for day := startOfDay(dueOn); days[dateKey(day)] > 0; day = day.AddDate(0, 0, -1) {
		count++
	}
	return count
}

func startOfDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func endOfDay(value time.Time) time.Time {
	return startOfDay(value).AddDate(0, 0, 1).Add(-time.Nanosecond)
}

func dateKey(value time.Time) string {
	return value.UTC().Format(time.DateOnly)
}
