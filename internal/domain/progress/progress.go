package progress

import (
	"errors"
	"math"
	"strings"
	"time"
)

var (
	ErrInvalidItem    = errors.New("progress item is invalid")
	ErrInvalidGrade   = errors.New("review grade is invalid")
	ErrNotFound       = errors.New("progress item not found")
	ErrConflict       = errors.New("progress item changed concurrently")
	ErrStorageFailure = errors.New("progress storage failed")
)

// MaxIntervalDays caps how far out a review can be scheduled. Uncapped, the
// interval compounds by the ease factor on every review and the due date
// eventually overflows the four-digit year RFC 3339 storage can round-trip.
const MaxIntervalDays = 365

type ItemKind string

const (
	KindVocab   ItemKind = "vocab"
	KindGrammar ItemKind = "grammar"
)

type Grade string

const (
	GradeAgain Grade = "again"
	GradeHard  Grade = "hard"
	GradeGood  Grade = "good"
	GradeEasy  Grade = "easy"
)

type Item struct {
	ID                  string
	Language            string
	Kind                ItemKind
	Headword            string
	Reading             string
	Gloss               string
	Example             string
	ExampleMeaning      string
	EaseFactor          float64
	IntervalDays        int
	Repetitions         int
	DueDate             time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	LastReviewedAt      time.Time
	LastLessonAttemptID string
	Version             int
}

type LessonActivity struct {
	AttemptID   string
	LessonID    string
	Language    string
	Title       string
	Score       int
	MaxScore    int
	CompletedAt time.Time
}

type ReviewActivity struct {
	ItemID     string
	Language   string
	Grade      Grade
	ReviewedAt time.Time
	ReviewedOn time.Time
}

func NewItem(candidate Item, now time.Time) (Item, error) {
	item := candidate
	item.ID = strings.TrimSpace(item.ID)
	item.Language = strings.TrimSpace(item.Language)
	item.Headword = strings.TrimSpace(item.Headword)
	item.Reading = strings.TrimSpace(item.Reading)
	item.Gloss = strings.TrimSpace(item.Gloss)
	if item.ID == "" || item.Language == "" || item.Headword == "" || !KnownKind(item.Kind) {
		return Item{}, ErrInvalidItem
	}
	if item.EaseFactor == 0 {
		item.EaseFactor = 2.5
	}
	if item.EaseFactor < 1.3 || item.IntervalDays < 0 || item.Repetitions < 0 || item.Version < 0 {
		return Item{}, ErrInvalidItem
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now.UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now.UTC()
	}
	return item, nil
}

func KnownKind(kind ItemKind) bool {
	return kind == KindVocab || kind == KindGrammar
}

func KnownGrade(grade Grade) bool {
	return grade == GradeAgain || grade == GradeHard || grade == GradeGood || grade == GradeEasy
}

func GradePerformance(correct, total int) Grade {
	if total <= 0 || correct <= 0 {
		return GradeAgain
	}
	ratio := float64(correct) / float64(total)
	switch {
	case ratio < 0.6:
		return GradeAgain
	case ratio < 0.8:
		return GradeHard
	case ratio < 0.95:
		return GradeGood
	default:
		return GradeEasy
	}
}

func Schedule(item Item, grade Grade, reviewedAt, reviewedOn time.Time) (Item, error) {
	if !KnownGrade(grade) {
		return Item{}, ErrInvalidGrade
	}
	if _, err := NewItem(item, reviewedAt); err != nil {
		return Item{}, err
	}

	switch grade {
	case GradeAgain:
		item.Repetitions = 0
		item.IntervalDays = 1
		item.EaseFactor = max(1.3, item.EaseFactor-0.2)
	case GradeHard:
		item.Repetitions++
		item.IntervalDays = nextInterval(item, 1, 3, 0.8)
		item.EaseFactor = max(1.3, item.EaseFactor-0.15)
	case GradeGood:
		item.Repetitions++
		item.IntervalDays = nextInterval(item, 1, 6, 1)
	case GradeEasy:
		item.Repetitions++
		item.IntervalDays = nextInterval(item, 4, 10, 1.3)
		item.EaseFactor += 0.1
	}
	item.IntervalDays = min(item.IntervalDays, MaxIntervalDays)

	if reviewedOn.IsZero() {
		reviewedOn = reviewedAt
	}
	day := startOfDay(reviewedOn)
	item.DueDate = day.AddDate(0, 0, item.IntervalDays)
	item.UpdatedAt = reviewedAt.UTC()
	item.LastReviewedAt = reviewedAt.UTC()
	item.Version++
	return item, nil
}

func nextInterval(item Item, first, second int, multiplier float64) int {
	switch item.Repetitions {
	case 1:
		return first
	case 2:
		return second
	default:
		return max(1, int(math.Round(float64(max(1, item.IntervalDays))*item.EaseFactor*multiplier)))
	}
}

func startOfDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
