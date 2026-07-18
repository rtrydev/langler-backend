package lessons_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/application/lessons"
	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type fakeStore struct {
	saved   []outbound.LessonRecord
	saveErr error
	record  outbound.LessonRecord
	getErr  error
	page    outbound.LessonPage
	listErr error
	deleted []string
	delErr  error
	results []outbound.ResultRecord
}

func (f *fakeStore) Save(_ context.Context, record outbound.LessonRecord) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, record)
	return nil
}

func (f *fakeStore) Get(_ context.Context, _, id string) (outbound.LessonRecord, error) {
	if f.getErr != nil {
		return outbound.LessonRecord{}, f.getErr
	}
	return f.record, nil
}

func (f *fakeStore) List(_ context.Context, _ string, _ int, _ string) (outbound.LessonPage, error) {
	if f.listErr != nil {
		return outbound.LessonPage{}, f.listErr
	}
	return f.page, nil
}

func (f *fakeStore) Delete(_ context.Context, _, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeStore) SaveResult(_ context.Context, record outbound.ResultRecord) error {
	f.results = append(f.results, record)
	return nil
}

type fakeChecker struct {
	missingVocab   []string
	missingGrammar []string
	err            error
}

func (f *fakeChecker) MissingVocab(_ context.Context, _ reference.Language, _ []string) ([]string, error) {
	return f.missingVocab, f.err
}

func (f *fakeChecker) MissingGrammar(_ context.Context, _ reference.Language, _ []string) ([]string, error) {
	return f.missingGrammar, f.err
}

type fakeReader struct {
	vocab   outbound.VocabPage
	grammar outbound.GrammarPage
	err     error
}

func (f *fakeReader) Vocab(_ context.Context, _ outbound.VocabFilter) (outbound.VocabPage, error) {
	return f.vocab, f.err
}

func (f *fakeReader) Grammar(_ context.Context, _ outbound.GrammarFilter) (outbound.GrammarPage, error) {
	return f.grammar, f.err
}

func (f *fakeReader) Scripts(_ context.Context, _ outbound.ScriptFilter) (outbound.ScriptPage, error) {
	return outbound.ScriptPage{}, f.err
}

func newService(t *testing.T, store *fakeStore, checker *fakeChecker, reader *fakeReader) *lessons.Service {
	t.Helper()
	svc, err := lessons.NewService(store, checker, reader, store)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func validLesson() domain.Lesson {
	return domain.Lesson{
		SchemaVersion: domain.SchemaVersion,
		ID:            "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
		Language:      "ja",
		Level:         "N4",
		Title:         "Weekend plans in Kyoto",
		ReadingStage:  domain.StageConnected,
		Exercises: []domain.Exercise{
			{
				ID:              "ex-1",
				Type:            domain.TypeCloze,
				Points:          8,
				ReferencedVocab: []string{"N4#1416220"},
				Cloze: &domain.Cloze{
					Text:   "先週の{{1}}に行きました。",
					Blanks: []domain.Blank{{Index: 1, Answer: "週末"}},
				},
			},
			{
				ID:   "ex-2",
				Type: domain.TypeReading,
				Reading: &domain.Reading{
					Genre:   domain.GenreShortStory,
					Title:   "京都の週末",
					Passage: "先週の週末、京都へ行きました。",
					Questions: []domain.Question{
						{Question: "どこへ行きましたか。", Kind: domain.KindShortAnswer, Answer: "京都"},
					},
				},
			},
		},
	}
}

func TestImportSavesValidatedLesson(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	result, err := svc.Import(context.Background(), inbound.LessonImportCommand{
		Owner:       "user-1",
		ContentHash: "abc",
		Lesson:      validLesson(),
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !result.Created {
		t.Error("Created = false, want true")
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved %d records, want 1", len(store.saved))
	}
	if store.saved[0].Owner != "user-1" || store.saved[0].ContentHash != "abc" {
		t.Errorf("record = %+v", store.saved[0])
	}
	if store.saved[0].CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestImportRejectsInvalidLesson(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	candidate := validLesson()
	candidate.Title = ""
	_, err := svc.Import(context.Background(), inbound.LessonImportCommand{Owner: "user-1", Lesson: candidate})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestImportRejectsMissingReferenceIDs(t *testing.T) {
	t.Parallel()

	checker := &fakeChecker{missingVocab: []string{"N4#1416220"}}
	svc := newService(t, &fakeStore{}, checker, &fakeReader{})
	_, err := svc.Import(context.Background(), inbound.LessonImportCommand{Owner: "user-1", Lesson: validLesson()})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	if len(validation.Issues) != 1 {
		t.Fatalf("issues = %v, want 1", validation.Issues)
	}
	if validation.Issues[0].Path != "exercises[0].referencedVocab[0]" {
		t.Errorf("path = %q", validation.Issues[0].Path)
	}
}

func TestImportReturnsExistingOnDuplicate(t *testing.T) {
	t.Parallel()

	existing := outbound.LessonRecord{
		Owner:     "user-1",
		CreatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Lesson:    validLesson(),
	}
	store := &fakeStore{saveErr: domain.ErrAlreadyExists, record: existing}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	result, err := svc.Import(context.Background(), inbound.LessonImportCommand{Owner: "user-1", Lesson: validLesson()})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Created {
		t.Error("Created = true, want false")
	}
	if !result.Stored.CreatedAt.Equal(existing.CreatedAt) {
		t.Errorf("CreatedAt = %v, want existing %v", result.Stored.CreatedAt, existing.CreatedAt)
	}
}

func TestImportRequiresOwner(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	_, err := svc.Import(context.Background(), inbound.LessonImportCommand{Lesson: validLesson()})
	if !errors.Is(err, domain.ErrInvalidOwner) {
		t.Fatalf("error = %v, want ErrInvalidOwner", err)
	}
}

func TestGetValidatesID(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	_, err := svc.Get(context.Background(), inbound.LessonQuery{Owner: "user-1", ID: "nope"})
	if !errors.Is(err, domain.ErrInvalidLessonID) {
		t.Fatalf("error = %v, want ErrInvalidLessonID", err)
	}
}

func TestDeleteDelegatesToStore(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	err := svc.Delete(context.Background(), inbound.LessonQuery{
		Owner: "user-1",
		ID:    "3e2d5f6a-9d0b-4c1e-8a7f-2b6c9d3e1f00",
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v", store.deleted)
	}
}

func TestRecordSavesValidatedPerUserResult(t *testing.T) {
	t.Parallel()

	store := &fakeStore{record: outbound.LessonRecord{Lesson: validLesson()}}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	started := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	result := domain.Result{
		AttemptID:   "11111111-1111-4111-8111-111111111111",
		LessonID:    validLesson().ID,
		StartedAt:   started,
		CompletedAt: started.Add(time.Minute),
		Score:       8,
		MaxScore:    8,
		AutoScore:   8,
		AutoMax:     8,
		Exercises: []domain.ExerciseResult{
			{ExerciseID: "ex-1", Type: domain.TypeCloze, Grading: "auto", Score: 8, MaxScore: 8, Correct: 1, Total: 1},
			{ExerciseID: "ex-2", Type: domain.TypeReading, Grading: "auto", Score: 0, MaxScore: 0, Correct: 1, Total: 1},
		},
	}

	if _, err := svc.Record(context.Background(), inbound.LessonResultCommand{Owner: "user-1", Result: result}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(store.results) != 1 || store.results[0].Owner != "user-1" {
		t.Fatalf("results = %+v", store.results)
	}
}

func TestBuildConnectedPromptIncludesStoryAndReference(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		vocab: outbound.VocabPage{Entries: []reference.VocabEntry{
			{ID: "N4#1416220", Headword: "週末", Reading: "しゅうまつ", Gloss: []string{"weekend"}},
		}},
		grammar: outbound.GrammarPage{Topics: []reference.GrammarTopic{
			{ID: "N4#volitional", Name: "Volitional form", Description: "Expressing intent."},
		}},
	}
	svc := newService(t, &fakeStore{}, &fakeChecker{}, reader)
	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:         "ja",
		Level:            "N4",
		Topic:            "Weekend travel",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	prompt := result.Prompt
	for _, want := range []string{
		"short_story",
		"N4#1416220 | 週末",
		"N4#volitional | Volitional form",
		`"readingStage": "connected"`,
		"JLPT N4",
		"cloze, reading",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildFoundationalPromptOmitsStory(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:      "ja",
		Level:         "N5",
		ExerciseTypes: []string{"script_practice", "reading"},
		ReadingStage:  "foundational",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(result.Prompt, "## Story requirement") {
		t.Error("foundational prompt contains story requirement")
	}
	if !strings.Contains(result.Prompt, `"readingStage": "foundational"`) {
		t.Error("foundational prompt missing stage instruction")
	}
	if strings.Contains(result.Prompt, "Exercise types to use: script_practice, reading") {
		t.Error("foundational prompt still requests reading exercises")
	}
}

func TestBuildRejectsUnknownParameters(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	_, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:      "xx",
		Level:         "N4",
		ExerciseTypes: []string{"cloze", "quiz"},
	})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	paths := make(map[string]bool)
	for _, issue := range validation.Issues {
		paths[issue.Path] = true
	}
	if !paths["language"] || !paths["exerciseTypes[1]"] {
		t.Errorf("issues = %v, want language and exerciseTypes[1]", validation.Issues)
	}
}
