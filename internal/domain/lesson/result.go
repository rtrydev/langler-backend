package lesson

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidResult = errors.New("lesson result is invalid")

type Result struct {
	AttemptID   string
	LessonID    string
	StartedAt   time.Time
	CompletedAt time.Time
	Score       int
	MaxScore    int
	AutoScore   int
	AutoMax     int
	SelfScore   int
	SelfMax     int
	Exercises   []ExerciseResult
}

type ExerciseResult struct {
	ExerciseID string
	Type       ExerciseType
	Grading    string
	Score      int
	MaxScore   int
	Correct    int
	Total      int
}

func NewResult(candidate Result, source Lesson) (Result, error) {
	r := candidate
	if !uuidPattern.MatchString(r.AttemptID) {
		return Result{}, fmt.Errorf("%w: attempt id must be a UUID", ErrInvalidResult)
	}
	if r.LessonID != source.ID {
		return Result{}, fmt.Errorf("%w: lesson id does not match", ErrInvalidResult)
	}
	if r.StartedAt.IsZero() || r.CompletedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) {
		return Result{}, fmt.Errorf("%w: completion time must not precede start time", ErrInvalidResult)
	}
	if len(r.Exercises) != len(source.Exercises) {
		return Result{}, fmt.Errorf("%w: every lesson exercise must have one result", ErrInvalidResult)
	}

	byID := make(map[string]Exercise, len(source.Exercises))
	for _, exercise := range source.Exercises {
		byID[exercise.ID] = exercise
	}
	seen := make(map[string]bool, len(r.Exercises))
	score, maximum, autoScore, autoMax, selfScore, selfMax := 0, 0, 0, 0, 0, 0
	for _, outcome := range r.Exercises {
		exercise, ok := byID[outcome.ExerciseID]
		if !ok || seen[outcome.ExerciseID] {
			return Result{}, fmt.Errorf("%w: exercise result does not match the lesson", ErrInvalidResult)
		}
		seen[outcome.ExerciseID] = true
		if outcome.Type != exercise.Type || outcome.MaxScore != exercise.Points {
			return Result{}, fmt.Errorf("%w: exercise type or points do not match the lesson", ErrInvalidResult)
		}
		if outcome.Grading != "auto" && outcome.Grading != "self" {
			return Result{}, fmt.Errorf("%w: grading must be auto or self", ErrInvalidResult)
		}
		if outcome.Score < 0 || outcome.Score > outcome.MaxScore || outcome.Correct < 0 || outcome.Total < 0 || outcome.Correct > outcome.Total {
			return Result{}, fmt.Errorf("%w: exercise score is out of range", ErrInvalidResult)
		}
		score += outcome.Score
		maximum += outcome.MaxScore
		if outcome.Grading == "auto" {
			autoScore += outcome.Score
			autoMax += outcome.MaxScore
		} else {
			selfScore += outcome.Score
			selfMax += outcome.MaxScore
		}
	}
	if r.Score != score || r.MaxScore != maximum || r.AutoScore != autoScore || r.AutoMax != autoMax || r.SelfScore != selfScore || r.SelfMax != selfMax {
		return Result{}, fmt.Errorf("%w: summary scores do not match exercise results", ErrInvalidResult)
	}
	return r, nil
}
