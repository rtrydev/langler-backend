package progress_test

import (
	"context"
	"testing"
	"time"

	progressapp "github.com/rtrydev/langler-backend/internal/application/progress"
	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type fakeStore struct {
	items            map[string]progress.Item
	saved            []progress.Item
	lesson           progress.LessonActivity
	review           progress.ReviewActivity
	due              map[string][]progress.Item
	snapshot         outbound.ProgressSnapshot
	reviewCalls      int
	concurrentReview *progress.Item
	lessonCalls      int
	concurrentLesson *progress.Item
}

func (f *fakeStore) GetItems(_ context.Context, _, _ string, keys []string) (map[string]progress.Item, error) {
	result := map[string]progress.Item{}
	for _, key := range keys {
		if item, ok := f.items[key]; ok {
			result[key] = item
		}
	}
	return result, nil
}

func (f *fakeStore) SaveLesson(_ context.Context, _ string, items []progress.Item, activity progress.LessonActivity) error {
	f.lessonCalls++
	if f.concurrentLesson != nil && f.lessonCalls == 1 {
		concurrent := *f.concurrentLesson
		f.items[string(concurrent.Kind)+"#"+concurrent.ID] = concurrent
		return progress.ErrConflict
	}
	f.saved, f.lesson = items, activity
	return nil
}

func (f *fakeStore) DueItems(_ context.Context, _, language string, _ time.Time) ([]progress.Item, error) {
	return f.due[language], nil
}

func (f *fakeStore) SaveReview(_ context.Context, _ string, item progress.Item, activity progress.ReviewActivity) error {
	f.reviewCalls++
	if f.concurrentReview != nil && f.reviewCalls == 1 {
		concurrent := *f.concurrentReview
		f.items[string(concurrent.Kind)+"#"+concurrent.ID] = concurrent
		return progress.ErrConflict
	}
	f.saved, f.review = []progress.Item{item}, activity
	f.items[string(item.Kind)+"#"+item.ID] = item
	return nil
}

func (f *fakeStore) Snapshot(context.Context, string) (outbound.ProgressSnapshot, error) {
	return f.snapshot, nil
}

type fakeReferences map[string]outbound.ReferenceContext

func (f fakeReferences) LookupProgress(context.Context, string, []string, []string) (map[string]outbound.ReferenceContext, error) {
	return f, nil
}

func newService(t *testing.T, store *fakeStore, references fakeReferences) *progressapp.Service {
	t.Helper()
	service, err := progressapp.NewService(store, references)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func TestRecordLessonSchedulesReferencedItems(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	service := newService(t, store, fakeReferences{
		"vocab#N4#1416220": {
			ID: "N4#1416220", Kind: progress.KindVocab, Headword: "週末", Reading: "しゅうまつ", Gloss: "weekend",
		},
		"grammar#N4#volitional": {
			ID: "N4#volitional", Kind: progress.KindGrammar, Headword: "Volitional form", Gloss: "Expressing intent",
		},
	})
	completedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	source := lesson.Lesson{
		ID: "11111111-1111-4111-8111-111111111111", Language: "ja", Title: "Kyoto",
		Exercises: []lesson.Exercise{
			{ID: "one", ReferencedVocab: []string{"N4#1416220"}},
			{ID: "two", ReferencedVocab: []string{"N4#1416220"}, ReferencedGrammar: []string{"N4#volitional"}},
		},
	}
	result := lesson.Result{
		AttemptID: "22222222-2222-4222-8222-222222222222", LessonID: source.ID,
		CompletedAt: completedAt, Score: 7, MaxScore: 10,
		Exercises: []lesson.ExerciseResult{
			{ExerciseID: "one", Correct: 1, Total: 1},
			{ExerciseID: "two", Correct: 0, Total: 1},
		},
	}

	if err := service.RecordLesson(context.Background(), "user-1", source, result, completedAt); err != nil {
		t.Fatalf("RecordLesson: %v", err)
	}
	if len(store.saved) != 2 {
		t.Fatalf("saved = %+v", store.saved)
	}
	for _, item := range store.saved {
		if item.IntervalDays != 1 || item.EaseFactor != 2.3 {
			t.Errorf("scheduled item = %+v", item)
		}
	}
	if store.lesson.LessonID != source.ID || store.lesson.AttemptID != result.AttemptID {
		t.Errorf("activity = %+v", store.lesson)
	}
	first := append([]progress.Item(nil), store.saved...)
	store.items = map[string]progress.Item{}
	for _, item := range first {
		store.items[string(item.Kind)+"#"+item.ID] = item
	}
	if err := service.RecordLesson(context.Background(), "user-1", source, result, completedAt); err != nil {
		t.Fatalf("duplicate RecordLesson: %v", err)
	}
	for i, item := range store.saved {
		if item.IntervalDays != first[i].IntervalDays || item.Repetitions != first[i].Repetitions {
			t.Errorf("duplicate rescheduled item: before=%+v after=%+v", first[i], item)
		}
	}
}

func TestRecordLessonSkipsReferencesWithoutAGrade(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	service := newService(t, store, fakeReferences{})
	completedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	source := lesson.Lesson{
		ID: "11111111-1111-4111-8111-111111111111", Language: "ja", Title: "Kyoto",
		Exercises: []lesson.Exercise{{ID: "reading", ReferencedVocab: []string{"N4#1416220"}}},
	}
	result := lesson.Result{
		AttemptID: "22222222-2222-4222-8222-222222222222", LessonID: source.ID, CompletedAt: completedAt,
		Exercises: []lesson.ExerciseResult{{ExerciseID: "reading", Correct: 0, Total: 0}},
	}

	if err := service.RecordLesson(context.Background(), "user-1", source, result, completedAt); err != nil {
		t.Fatalf("RecordLesson: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved = %+v", store.saved)
	}
	if store.lesson.AttemptID != result.AttemptID {
		t.Fatalf("activity = %+v", store.lesson)
	}
}

func TestRecordLessonRetriesAfterConcurrentUpdate(t *testing.T) {
	t.Parallel()

	completedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	item := progress.Item{
		ID: "N4#1416220", Language: "ja", Kind: progress.KindVocab, Headword: "週末", Gloss: "weekend",
		EaseFactor: 2.5, IntervalDays: 1, Repetitions: 1, Version: 1,
		CreatedAt: completedAt.AddDate(0, 0, -1), UpdatedAt: completedAt.AddDate(0, 0, -1),
	}
	concurrent := item
	concurrent.IntervalDays = 6
	concurrent.Repetitions = 2
	concurrent.Version = 2
	store := &fakeStore{
		items:            map[string]progress.Item{"vocab#N4#1416220": item},
		concurrentLesson: &concurrent,
	}
	service := newService(t, store, fakeReferences{})
	source := lesson.Lesson{
		ID: "11111111-1111-4111-8111-111111111111", Language: "ja", Title: "Kyoto",
		Exercises: []lesson.Exercise{{ID: "one", ReferencedVocab: []string{"N4#1416220"}}},
	}
	result := lesson.Result{
		AttemptID: "22222222-2222-4222-8222-222222222222", LessonID: source.ID, CompletedAt: completedAt,
		Exercises: []lesson.ExerciseResult{{ExerciseID: "one", Correct: 9, Total: 10}},
	}

	if err := service.RecordLesson(context.Background(), "user-1", source, result, completedAt); err != nil {
		t.Fatalf("RecordLesson: %v", err)
	}
	if store.lessonCalls != 2 || len(store.saved) != 1 || store.saved[0].IntervalDays != 15 || store.saved[0].Version != 3 {
		t.Fatalf("saved = %+v, calls = %d", store.saved, store.lessonCalls)
	}
}

func TestGradeMovesItemToItsNextDueDate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 19, 15, 0, 0, 0, time.UTC)
	item, err := progress.NewItem(progress.Item{
		ID: "N4#1416220", Language: "ja", Kind: progress.KindVocab, Headword: "週末", Gloss: "weekend",
	}, now)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	store := &fakeStore{items: map[string]progress.Item{"vocab#N4#1416220": item}}
	service := newService(t, store, fakeReferences{})

	updated, err := service.Grade(context.Background(), inbound.ReviewGradeCommand{
		Owner: "user-1", Language: "ja", Kind: progress.KindVocab,
		ItemID: item.ID, Grade: progress.GradeGood, ReviewedAt: now,
	})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if !updated.DueDate.Equal(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("DueDate = %v", updated.DueDate)
	}
	if store.review.Grade != progress.GradeGood {
		t.Errorf("review = %+v", store.review)
	}
}

func TestGradeRetriesAfterConcurrentUpdate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 19, 15, 0, 0, 0, time.UTC)
	item := progress.Item{
		ID: "N4#1416220", Language: "ja", Kind: progress.KindVocab, Headword: "週末", Gloss: "weekend",
		EaseFactor: 2.5, IntervalDays: 1, Repetitions: 1, Version: 1,
		CreatedAt: now.AddDate(0, 0, -1), UpdatedAt: now.AddDate(0, 0, -1),
	}
	concurrent := item
	concurrent.IntervalDays = 6
	concurrent.Repetitions = 2
	concurrent.Version = 2
	store := &fakeStore{
		items:            map[string]progress.Item{"vocab#N4#1416220": item},
		concurrentReview: &concurrent,
	}
	service := newService(t, store, fakeReferences{})

	updated, err := service.Grade(context.Background(), inbound.ReviewGradeCommand{
		Owner: "user-1", Language: "ja", Kind: progress.KindVocab,
		ItemID: item.ID, Grade: progress.GradeGood, ReviewedAt: now, ReviewedOn: now,
	})
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if store.reviewCalls != 2 || updated.IntervalDays != 15 || updated.Version != 3 {
		t.Fatalf("updated = %+v, calls = %d", updated, store.reviewCalls)
	}
}

func TestSummaryReflectsActivityByLanguage(t *testing.T) {
	t.Parallel()

	today := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{snapshot: outbound.ProgressSnapshot{
		Items: []progress.Item{
			{Language: "ja", DueDate: today.Add(-time.Hour)},
			{Language: "ja", DueDate: today.AddDate(0, 0, 2)},
		},
		LessonActivity: []progress.LessonActivity{
			{LessonID: "lesson-1", Language: "ja", Title: "First", CompletedAt: today.Add(-time.Hour)},
			{LessonID: "lesson-1", Language: "ja", Title: "First", CompletedAt: today.Add(-2 * time.Hour)},
			{LessonID: "lesson-2", Language: "ja", Title: "Second", CompletedAt: today.Add(-3 * time.Hour)},
		},
		ReviewActivity: []progress.ReviewActivity{
			{Language: "ja", ReviewedAt: today},
			{Language: "ja", ReviewedAt: today.AddDate(0, 0, -1)},
		},
	}}
	service := newService(t, store, fakeReferences{})

	summary, err := service.Summary(context.Background(), inbound.ProgressSummaryQuery{Owner: "user-1", DueOn: today})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	stats := summary.Languages[0]
	if stats.Language != "ja" || stats.LessonsCompleted != 2 || stats.ItemsTracked != 2 || stats.DueToday != 1 || stats.CurrentReviewStreak != 2 {
		t.Errorf("stats = %+v", stats)
	}
	if len(stats.RecentLessons) != 3 || len(stats.ReviewHistory) != 2 {
		t.Errorf("activity = %+v", stats)
	}
}
