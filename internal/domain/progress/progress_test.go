package progress_test

import (
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/progress"
)

func TestScheduleFollowsSM2StyleIntervals(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	item, err := progress.NewItem(progress.Item{
		ID: "N4#1416220", Language: "ja", Kind: progress.KindVocab,
		Headword: "週末", Reading: "しゅうまつ", Gloss: "weekend",
	}, now)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}

	item, err = progress.Schedule(item, progress.GradeGood, now, now)
	if err != nil {
		t.Fatalf("first Schedule: %v", err)
	}
	if item.IntervalDays != 1 || item.Repetitions != 1 {
		t.Fatalf("first schedule = %+v", item)
	}

	item, err = progress.Schedule(item, progress.GradeGood, item.DueDate.Add(12*time.Hour), item.DueDate)
	if err != nil {
		t.Fatalf("second Schedule: %v", err)
	}
	if item.IntervalDays != 6 || item.Repetitions != 2 {
		t.Fatalf("second schedule = %+v", item)
	}

	item, err = progress.Schedule(item, progress.GradeEasy, item.DueDate.Add(12*time.Hour), item.DueDate)
	if err != nil {
		t.Fatalf("third Schedule: %v", err)
	}
	if item.IntervalDays != 20 || item.EaseFactor != 2.6 {
		t.Fatalf("easy schedule = %+v", item)
	}

	item, err = progress.Schedule(item, progress.GradeAgain, item.DueDate.Add(12*time.Hour), item.DueDate)
	if err != nil {
		t.Fatalf("again Schedule: %v", err)
	}
	if item.IntervalDays != 1 || item.Repetitions != 0 || item.EaseFactor != 2.4 {
		t.Fatalf("again schedule = %+v", item)
	}
}

func TestGradePerformance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		correct int
		total   int
		want    progress.Grade
	}{
		{0, 4, progress.GradeAgain},
		{2, 4, progress.GradeAgain},
		{3, 4, progress.GradeHard},
		{9, 10, progress.GradeGood},
		{4, 4, progress.GradeEasy},
	}
	for _, test := range tests {
		if got := progress.GradePerformance(test.correct, test.total); got != test.want {
			t.Errorf("GradePerformance(%d, %d) = %q, want %q", test.correct, test.total, got, test.want)
		}
	}
}

func TestScheduleUsesTheLearnersCalendarDate(t *testing.T) {
	t.Parallel()

	reviewedAt := time.Date(2026, 7, 18, 22, 30, 0, 0, time.UTC)
	reviewedOn := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	item, err := progress.NewItem(progress.Item{
		ID: "N4#1416220", Language: "ja", Kind: progress.KindVocab, Headword: "週末", Gloss: "weekend",
	}, reviewedAt)
	if err != nil {
		t.Fatalf("NewItem: %v", err)
	}
	item, err = progress.Schedule(item, progress.GradeGood, reviewedAt, reviewedOn)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if want := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC); !item.DueDate.Equal(want) {
		t.Fatalf("DueDate = %v, want %v", item.DueDate, want)
	}
	if item.Version != 1 {
		t.Fatalf("Version = %d, want 1", item.Version)
	}
}
