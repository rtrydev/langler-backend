package lesson_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/rtrydev/langler-backend/internal/domain/lesson"
)

func validLesson() lesson.Lesson {
	return lesson.Lesson{
		SchemaVersion:    lesson.SchemaVersion,
		ID:               "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		Language:         "ja",
		Level:            "N4",
		Title:            "Weekend plans in Kyoto",
		Description:      "Travel and daily routines.",
		Topic:            "Travel",
		Tags:             []string{"travel", "kyoto"},
		ReadingStage:     lesson.StageConnected,
		SourceModel:      "Claude",
		EstimatedMinutes: 18,
		Exercises: []lesson.Exercise{
			{
				ID:              "ex-1",
				Type:            lesson.TypeCloze,
				Prompt:          "Fill in the blanks.",
				Points:          8,
				ReferencedVocab: []string{"N4#1416220"},
				Cloze: &lesson.Cloze{
					Text: "先週の{{1}}、友達と京都へ{{2}}ました。",
					Blanks: []lesson.Blank{
						{Index: 1, Answer: "週末"},
						{Index: 2, Answer: "行き", Hint: "polite past"},
					},
				},
			},
			{
				ID:     "ex-2",
				Type:   lesson.TypeReading,
				Prompt: "Read the story and answer the questions.",
				Points: 12,
				Reading: &lesson.Reading{
					Genre:   lesson.GenreShortStory,
					Title:   "京都の週末",
					Passage: "先週の週末、友達と京都へ行きました。お寺をたくさん見て、抹茶を飲みました。",
					Annotations: []lesson.Annotation{
						{Surface: "京都", Reading: "きょうと", Gloss: "Kyoto"},
					},
					Questions: []lesson.Question{
						{
							Question: "京都で何を飲みましたか。",
							Kind:     lesson.KindMultipleChoice,
							Options:  []string{"コーヒー", "抹茶"},
							Answer:   "抹茶",
						},
						{
							Question: "だれと行きましたか。",
							Kind:     lesson.KindShortAnswer,
							Answer:   "友達と行きました。",
						},
					},
				},
			},
		},
	}
}

func issuePaths(t *testing.T, err error) []string {
	t.Helper()
	var validation *lesson.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want *lesson.ValidationError", err)
	}
	paths := make([]string, 0, len(validation.Issues))
	for _, issue := range validation.Issues {
		paths = append(paths, issue.Path)
	}
	return paths
}

func TestNewAcceptsValidLesson(t *testing.T) {
	t.Parallel()

	l, err := lesson.New(validLesson())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l.Title != "Weekend plans in Kyoto" {
		t.Errorf("Title = %q", l.Title)
	}
}

func TestNewTrimsWhitespace(t *testing.T) {
	t.Parallel()

	candidate := validLesson()
	candidate.Title = "  Weekend plans  "
	l, err := lesson.New(candidate)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l.Title != "Weekend plans" {
		t.Errorf("Title = %q, want trimmed", l.Title)
	}
}

func TestNewCollectsIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*lesson.Lesson)
		wantPath string
	}{
		{
			name:     "wrong schema version",
			mutate:   func(l *lesson.Lesson) { l.SchemaVersion = "2.0" },
			wantPath: "schemaVersion",
		},
		{
			name:     "invalid lesson id",
			mutate:   func(l *lesson.Lesson) { l.ID = "not-a-uuid" },
			wantPath: "lessonId",
		},
		{
			name:     "unknown language",
			mutate:   func(l *lesson.Lesson) { l.Language = "xx" },
			wantPath: "language",
		},
		{
			name:     "level not valid for language",
			mutate:   func(l *lesson.Lesson) { l.Level = "B1" },
			wantPath: "level",
		},
		{
			name:     "missing title",
			mutate:   func(l *lesson.Lesson) { l.Title = "" },
			wantPath: "title",
		},
		{
			name:     "unknown reading stage",
			mutate:   func(l *lesson.Lesson) { l.ReadingStage = "beginner" },
			wantPath: "readingStage",
		},
		{
			name:     "no exercises",
			mutate:   func(l *lesson.Lesson) { l.Exercises = nil },
			wantPath: "exercises",
		},
		{
			name: "connected lesson without story",
			mutate: func(l *lesson.Lesson) {
				l.Exercises = l.Exercises[:1]
			},
			wantPath: "exercises",
		},
		{
			name: "story without questions",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[1].Reading.Questions = nil
			},
			wantPath: "exercises[1].payload.questions",
		},
		{
			name: "unknown exercise type",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].Type = "reading_comp"
			},
			wantPath: "exercises[0].type",
		},
		{
			name: "duplicate exercise ids",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[1].ID = "ex-1"
			},
			wantPath: "exercises[1].exerciseId",
		},
		{
			name: "cloze blank without answer",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].Cloze.Blanks[1].Answer = ""
			},
			wantPath: "exercises[0].payload.blanks[1].answer",
		},
		{
			name: "cloze blank without marker",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].Cloze.Blanks[1].Index = 9
			},
			wantPath: "exercises[0].payload.blanks[1].index",
		},
		{
			name: "cloze marker without blank",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].Cloze.Text += "そして{{3}}。"
			},
			wantPath: "exercises[0].payload.blanks",
		},
		{
			name: "missing payload",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].Cloze = nil
			},
			wantPath: "exercises[0].payload",
		},
		{
			name: "bad reference id format",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].ReferencedVocab = []string{"lowercase#bad level"}
			},
			wantPath: "exercises[0].referencedVocab[0]",
		},
		{
			name: "too many scheduled references",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].ReferencedVocab = make([]string, 100)
				for index := range l.Exercises[0].ReferencedVocab {
					l.Exercises[0].ReferencedVocab[index] = "N4#item-" + strconv.Itoa(index)
				}
			},
			wantPath: "exercises",
		},
		{
			name: "multiple choice answer not among options",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[1].Reading.Questions[0].Answer = "お茶"
			},
			wantPath: "exercises[1].payload.questions[0].answer",
		},
		{
			name: "control characters rejected",
			mutate: func(l *lesson.Lesson) {
				l.Description = "bad\x00text"
			},
			wantPath: "description",
		},
		{
			name: "html rejected",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[1].Reading.Passage = "<script>alert('x')</script>先週の週末、京都へ行きました。"
			},
			wantPath: "exercises[1].payload.passage",
		},
		{
			name: "oversized passage rejected",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[1].Reading.Passage = strings.Repeat("あ", 6001)
			},
			wantPath: "exercises[1].payload.passage",
		},
		{
			name: "japanese script required in cloze",
			mutate: func(l *lesson.Lesson) {
				l.Exercises[0].Cloze.Text = "I went to {{1}} with my {{2}}."
			},
			wantPath: "exercises[0].payload.text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			candidate := validLesson()
			tt.mutate(&candidate)
			_, err := lesson.New(candidate)
			if err == nil {
				t.Fatal("New: error = nil, want validation error")
			}
			paths := issuePaths(t, err)
			for _, path := range paths {
				if path == tt.wantPath {
					return
				}
			}
			t.Errorf("issue paths = %v, want to include %q", paths, tt.wantPath)
		})
	}
}

func TestNewAllowsFoundationalWithoutStory(t *testing.T) {
	t.Parallel()

	candidate := validLesson()
	candidate.ReadingStage = lesson.StageFoundational
	candidate.Exercises = []lesson.Exercise{
		{
			ID:   "ex-1",
			Type: lesson.TypeScriptPractice,
			ScriptPractice: &lesson.ScriptPractice{
				Items: []lesson.ScriptItem{{Glyph: "京", Reading: "きょう", Meaning: "capital"}},
			},
		},
	}
	if _, err := lesson.New(candidate); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewReportsAllIssuesAtOnce(t *testing.T) {
	t.Parallel()

	candidate := validLesson()
	candidate.Title = ""
	candidate.ID = "nope"
	candidate.Exercises[0].Cloze.Blanks[0].Answer = ""
	_, err := lesson.New(candidate)
	if err == nil {
		t.Fatal("New: error = nil")
	}
	if paths := issuePaths(t, err); len(paths) < 3 {
		t.Errorf("issue count = %d (%v), want all issues collected", len(paths), paths)
	}
}
