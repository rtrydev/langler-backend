package lesson_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
)

func TestNewResultValidatesBreakdownAgainstLesson(t *testing.T) {
	t.Parallel()

	source, valid := validResult()
	tests := []struct {
		name   string
		change func(*lesson.Result)
	}{
		{
			name: "summary mismatch",
			change: func(result *lesson.Result) {
				result.Score = 9
			},
		},
		{
			name: "score contradicts correct count",
			change: func(result *lesson.Result) {
				result.Score = 10
				result.AutoScore = 8
				result.Exercises[0].Score = 8
			},
		},
		{
			name: "total contradicts exercise",
			change: func(result *lesson.Result) {
				result.Exercises[0].Total = 5
			},
		},
		{
			name: "grading contradicts exercise type",
			change: func(result *lesson.Result) {
				result.AutoScore = 0
				result.AutoMax = 0
				result.SelfScore = result.Score
				result.SelfMax = result.MaxScore
				result.Exercises[0].Grading = "self"
			},
		},
	}

	if _, err := lesson.NewResult(valid, source); err != nil {
		t.Fatalf("NewResult(valid): %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			candidate.Exercises = append([]lesson.ExerciseResult(nil), valid.Exercises...)
			test.change(&candidate)
			if _, err := lesson.NewResult(candidate, source); !errors.Is(err, lesson.ErrInvalidResult) {
				t.Fatalf("NewResult() error = %v, want ErrInvalidResult", err)
			}
		})
	}
}

func TestNewResultGradesMultipleChoiceAutomatically(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	source := lesson.Lesson{
		ID: "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		Exercises: []lesson.Exercise{
			{
				ID:     "ex-1",
				Type:   lesson.TypeMultipleChoice,
				Points: 6,
				MultipleChoice: &lesson.MultipleChoice{
					Questions: []lesson.MCQuestion{
						{Question: "q1", Options: []string{"a", "b"}, Answer: "a"},
						{Question: "q2", Options: []string{"a", "b"}, Answer: "b"},
						{Question: "q3", Options: []string{"a", "b"}, Answer: "a"},
					},
				},
			},
		},
	}
	result := lesson.Result{
		AttemptID:   "11111111-1111-4111-8111-111111111111",
		LessonID:    source.ID,
		StartedAt:   started,
		CompletedAt: started.Add(time.Minute),
		Score:       4,
		MaxScore:    6,
		AutoScore:   4,
		AutoMax:     6,
		Exercises: []lesson.ExerciseResult{
			{ExerciseID: "ex-1", Type: lesson.TypeMultipleChoice, Grading: "auto", Score: 4, MaxScore: 6, Correct: 2, Total: 3},
		},
	}
	if _, err := lesson.NewResult(result, source); err != nil {
		t.Fatalf("NewResult: %v", err)
	}

	result.Exercises[0].Grading = "self"
	result.SelfScore, result.SelfMax, result.AutoScore, result.AutoMax = 4, 6, 0, 0
	if _, err := lesson.NewResult(result, source); !errors.Is(err, lesson.ErrInvalidResult) {
		t.Fatalf("NewResult(self-graded multiple choice) error = %v, want ErrInvalidResult", err)
	}
}

func TestNewResultGradesPolishOrthographyAutomatically(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	source := lesson.Lesson{
		ID: "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		Exercises: []lesson.Exercise{{
			ID:     "ort-1",
			Type:   lesson.TypeScriptPractice,
			Points: 6,
			ScriptPractice: &lesson.ScriptPractice{Items: []lesson.ScriptItem{
				{Kind: lesson.ScriptKindChoice, Answer: "król", Options: []string{"król", "krul"}},
				{Kind: lesson.ScriptKindDictation, Answer: "morze"},
			}},
		}},
	}
	result := lesson.Result{
		AttemptID:   "11111111-1111-4111-8111-111111111111",
		LessonID:    source.ID,
		StartedAt:   started,
		CompletedAt: started.Add(time.Minute),
		Score:       3,
		MaxScore:    6,
		AutoScore:   3,
		AutoMax:     6,
		Exercises: []lesson.ExerciseResult{{
			ExerciseID: "ort-1", Type: lesson.TypeScriptPractice, Grading: "auto",
			Score: 3, MaxScore: 6, Correct: 1, Total: 2,
		}},
	}
	if _, err := lesson.NewResult(result, source); err != nil {
		t.Fatalf("NewResult: %v", err)
	}
}

func validResult() (lesson.Lesson, lesson.Result) {
	started := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	source := lesson.Lesson{
		ID: "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		Exercises: []lesson.Exercise{
			{
				ID:     "ex-1",
				Type:   lesson.TypeCloze,
				Points: 8,
				Cloze: &lesson.Cloze{
					Text: "{{1}} {{2}} {{3}} {{4}}",
					Blanks: []lesson.Blank{
						{Index: 1, Answer: "one"},
						{Index: 2, Answer: "two"},
						{Index: 3, Answer: "three"},
						{Index: 4, Answer: "four"},
					},
				},
			},
			{
				ID:          "ex-2",
				Type:        lesson.TypeTranslation,
				Points:      8,
				Translation: &lesson.Translation{Source: "Hello"},
			},
		},
	}
	result := lesson.Result{
		AttemptID:   "11111111-1111-4111-8111-111111111111",
		LessonID:    source.ID,
		StartedAt:   started,
		CompletedAt: started.Add(time.Minute),
		Score:       8,
		MaxScore:    16,
		AutoScore:   6,
		AutoMax:     8,
		SelfScore:   2,
		SelfMax:     8,
		Exercises: []lesson.ExerciseResult{
			{ExerciseID: "ex-1", Type: lesson.TypeCloze, Grading: "auto", Score: 6, MaxScore: 8, Correct: 3, Total: 4},
			{ExerciseID: "ex-2", Type: lesson.TypeTranslation, Grading: "self", Score: 2, MaxScore: 8, Correct: 1, Total: 4},
		},
	}
	return source, result
}
