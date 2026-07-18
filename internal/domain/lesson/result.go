package lesson

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var ErrInvalidResult = errors.New("lesson result is invalid")

const (
	gradingAuto = "auto"
	gradingSelf = "self"
)

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
		if outcome.Score < 0 || outcome.Score > outcome.MaxScore || outcome.Correct < 0 || outcome.Total < 0 || outcome.Correct > outcome.Total {
			return Result{}, fmt.Errorf("%w: exercise score is out of range", ErrInvalidResult)
		}
		grading, total := resultScale(exercise)
		if outcome.Grading != grading {
			return Result{}, fmt.Errorf("%w: exercise grading does not match its type", ErrInvalidResult)
		}
		if outcome.Total != total || outcome.Score != scoreFor(exercise.Points, outcome.Correct, total) {
			return Result{}, fmt.Errorf("%w: exercise score does not match its outcome", ErrInvalidResult)
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

func resultScale(exercise Exercise) (string, int) {
	switch exercise.Type {
	case TypeCloze:
		if exercise.Cloze == nil {
			return gradingAuto, 0
		}
		return gradingAuto, len(exercise.Cloze.Blanks)
	case TypeOrdering:
		if exercise.Ordering == nil {
			return gradingAuto, 0
		}
		return gradingAuto, len(exercise.Ordering.Items)
	case TypeMatching:
		if exercise.Matching == nil {
			return gradingAuto, 0
		}
		return gradingAuto, len(exercise.Matching.Pairs)
	case TypeReading:
		total := 0
		if exercise.Reading != nil {
			for _, question := range exercise.Reading.Questions {
				if strings.TrimSpace(question.Answer) != "" {
					total++
				}
			}
		}
		return gradingAuto, total
	case TypeTranslation, TypeWritingPrompt, TypeScriptPractice:
		return gradingSelf, 4
	default:
		return "", 0
	}
}

func scoreFor(points, correct, total int) int {
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(points) * float64(correct) / float64(total)))
}
