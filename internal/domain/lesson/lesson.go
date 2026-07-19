package lesson

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const SchemaVersion = "1.0"

var (
	ErrNotFound            = errors.New("lesson not found")
	ErrAlreadyExists       = errors.New("lesson already exists")
	ErrStorageFailure      = errors.New("lesson storage failed")
	ErrInvalidOwner        = errors.New("owner must not be empty")
	ErrInvalidLessonID     = errors.New("lesson id must be a UUID")
	ErrInvalidCursor       = errors.New("cursor is not a valid pagination token")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for different lesson content")
)

type Language string

type Level string

type ReadingStage string

const (
	StageConnected    ReadingStage = "connected"
	StageFoundational ReadingStage = "foundational"
)

func Languages() []Language {
	return []Language{"ja", "my", "pl"}
}

func LevelsFor(language Language) []Level {
	switch language {
	case "ja":
		return []Level{"N5", "N4", "N3", "N2", "N1"}
	case "my", "pl":
		return []Level{"A1", "A2", "B1", "B2", "C1", "C2"}
	default:
		return nil
	}
}

func KnownLanguage(language Language) bool {
	return slices.Contains(Languages(), language)
}

func KnownLevel(language Language, level Level) bool {
	return slices.Contains(LevelsFor(language), level)
}

func KnownStage(stage ReadingStage) bool {
	return stage == StageConnected || stage == StageFoundational
}

type Lesson struct {
	SchemaVersion    string
	ID               string
	Language         Language
	Level            Level
	Title            string
	Description      string
	Topic            string
	Tags             []string
	ReadingStage     ReadingStage
	SourceModel      string
	EstimatedMinutes int
	Exercises        []Exercise
}

const (
	maxTitleLen        = 160
	maxDescriptionLen  = 1000
	maxTopicLen        = 120
	maxTags            = 10
	maxTagLen          = 40
	maxSourceModelLen  = 80
	maxMinutes         = 240
	maxExercises       = 25
	maxReferencesInAll = 99
)

var scriptHooks = map[Language]func(*collector, Lesson){
	"ja": japaneseIssues,
}

func New(candidate Lesson) (Lesson, error) {
	l := candidate
	c := &collector{}

	l.SchemaVersion = strings.TrimSpace(l.SchemaVersion)
	if l.SchemaVersion != SchemaVersion {
		c.add("schemaVersion", "must be %q", SchemaVersion)
	}
	if !uuidPattern.MatchString(l.ID) {
		c.add("lessonId", "must be a UUID such as 3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00")
	}
	if !KnownLanguage(l.Language) {
		c.add("language", "must be one of %s", joinValues(Languages()))
	} else if !KnownLevel(l.Language, l.Level) {
		c.add("level", "must be one of %s for language %q", joinValues(LevelsFor(l.Language)), l.Language)
	}

	l.Title = strings.TrimSpace(l.Title)
	c.text("title", l.Title, maxTitleLen, true)
	l.Description = strings.TrimSpace(l.Description)
	c.text("description", l.Description, maxDescriptionLen, false)
	l.Topic = strings.TrimSpace(l.Topic)
	c.text("topic", l.Topic, maxTopicLen, false)

	if len(l.Tags) > maxTags {
		c.add("tags", "must contain at most %d tags", maxTags)
	}
	for i := range l.Tags {
		l.Tags[i] = strings.TrimSpace(l.Tags[i])
		c.text(fmt.Sprintf("tags[%d]", i), l.Tags[i], maxTagLen, true)
	}

	if !KnownStage(l.ReadingStage) {
		c.add("readingStage", "must be %q or %q", StageConnected, StageFoundational)
	}
	l.SourceModel = strings.TrimSpace(l.SourceModel)
	c.text("sourceModel", l.SourceModel, maxSourceModelLen, false)
	if l.EstimatedMinutes < 0 || l.EstimatedMinutes > maxMinutes {
		c.add("estimatedMinutes", "must be between 0 and %d", maxMinutes)
	}

	if len(l.Exercises) == 0 {
		c.add("exercises", "must contain at least one exercise")
	}
	if len(l.Exercises) > maxExercises {
		c.add("exercises", "must contain at most %d exercises", maxExercises)
	}

	seenIDs := map[string]bool{}
	references := 0
	for i := range l.Exercises {
		path := fmt.Sprintf("exercises[%d]", i)
		e := &l.Exercises[i]
		validateExercise(c, path, e)
		if e.ID != "" && seenIDs[e.ID] {
			c.add(path+".exerciseId", "duplicates another exercise id %q", e.ID)
		}
		seenIDs[e.ID] = true
		references += len(e.ReferencedVocab) + len(e.ReferencedGrammar)
	}
	if references > maxReferencesInAll {
		c.add("exercises", "must reference at most %d vocabulary and grammar items in total", maxReferencesInAll)
	}

	if l.ReadingStage == StageConnected && !opensWithShortStory(l) {
		c.add("exercises", "a %q lesson must open with a reading exercise as exercises[0] (genre %q, a title, a passage, and at least one comprehension question) so the story introduces the language before it is tested; set readingStage to %q only when the learner cannot decode connected text yet", StageConnected, GenreShortStory, StageFoundational)
	}

	if hook := scriptHooks[l.Language]; hook != nil {
		hook(c, l)
	}

	if err := c.err(); err != nil {
		return Lesson{}, err
	}
	return l, nil
}

func opensWithShortStory(l Lesson) bool {
	if len(l.Exercises) == 0 {
		return false
	}
	e := l.Exercises[0]
	r := e.Reading
	return e.Type == TypeReading && r != nil && r.Genre == GenreShortStory &&
		strings.TrimSpace(r.Title) != "" && strings.TrimSpace(r.Passage) != "" && len(r.Questions) > 0
}

func joinValues[T ~string](values []T) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", string(v)))
	}
	return strings.Join(quoted, ", ")
}
