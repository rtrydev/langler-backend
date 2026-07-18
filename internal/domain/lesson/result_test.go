package lesson_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
)

func TestNewResultValidatesBreakdownAgainstLesson(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	source := lesson.Lesson{
		ID:        "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		Exercises: []lesson.Exercise{{ID: "ex-1", Type: lesson.TypeCloze, Points: 8}},
	}
	result := lesson.Result{
		AttemptID:   "11111111-1111-4111-8111-111111111111",
		LessonID:    source.ID,
		StartedAt:   started,
		CompletedAt: started.Add(time.Minute),
		Score:       6,
		MaxScore:    8,
		AutoScore:   6,
		AutoMax:     8,
		Exercises: []lesson.ExerciseResult{{
			ExerciseID: "ex-1", Type: lesson.TypeCloze, Grading: "auto", Score: 6, MaxScore: 8, Correct: 3, Total: 4,
		}},
	}

	if _, err := lesson.NewResult(result, source); err != nil {
		t.Fatalf("NewResult: %v", err)
	}
	result.Score = 8
	if _, err := lesson.NewResult(result, source); !errors.Is(err, lesson.ErrInvalidResult) {
		t.Fatalf("mismatched score error = %v, want ErrInvalidResult", err)
	}
}
