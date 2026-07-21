package lessons_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rtrydev/langler-backend/internal/application/lessons"
	domain "github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/progress"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type fakeStore struct {
	saved       []outbound.LessonRecord
	saveErr     error
	record      outbound.LessonRecord
	getErr      error
	page        outbound.LessonPage
	listErr     error
	deleted     []string
	delErr      error
	results     []outbound.ResultRecord
	completions []outbound.Completion
	resultsErr  error
}

func (f *fakeStore) Save(_ context.Context, record outbound.LessonRecord) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, record)
	return nil
}

func (f *fakeStore) SaveIdempotent(_ context.Context, record outbound.LessonRecord, _ string) (outbound.LessonRecord, bool, error) {
	if f.saveErr != nil {
		if errors.Is(f.saveErr, domain.ErrAlreadyExists) {
			return f.record, false, nil
		}
		return outbound.LessonRecord{}, false, f.saveErr
	}
	f.saved = append(f.saved, record)
	return record, true, nil
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

func (f *fakeStore) ListResults(_ context.Context, _, lessonID string, limit int) ([]outbound.Completion, error) {
	if f.resultsErr != nil {
		return nil, f.resultsErr
	}
	var matched []outbound.Completion
	for _, completion := range f.completions {
		if completion.LessonID == lessonID && len(matched) < limit {
			matched = append(matched, completion)
		}
	}
	return matched, nil
}

func (f *fakeStore) ListCompletions(_ context.Context, _ string) ([]outbound.Completion, error) {
	if f.resultsErr != nil {
		return nil, f.resultsErr
	}
	return f.completions, nil
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
	topics  []reference.Topic
	byID    map[string]reference.VocabEntry
	err     error
}

type fakeSemantic struct {
	ids   []string
	err   error
	topic string
}

func (f *fakeSemantic) SimilarVocabIDs(_ context.Context, _ reference.Language, _ reference.Level, topic string, _ int) ([]string, error) {
	f.topic = topic
	return f.ids, f.err
}

type fakeCoverage struct {
	vocab   []string
	grammar []string
	err     error
}

func (f *fakeCoverage) CoveredItemIDs(_ context.Context, _, _ string, kind progress.ItemKind) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if kind == progress.KindGrammar {
		return f.grammar, nil
	}
	return f.vocab, nil
}

type fakeProgressRecorder struct {
	owner       string
	lesson      domain.Lesson
	result      domain.Result
	completedOn time.Time
	err         error
}

func (f *fakeProgressRecorder) RecordLesson(_ context.Context, owner string, source domain.Lesson, result domain.Result, completedOn time.Time) error {
	f.owner = owner
	f.lesson = source
	f.result = result
	f.completedOn = completedOn
	return f.err
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

func (f *fakeReader) Topics(_ context.Context, filter outbound.TopicFilter) ([]reference.Topic, error) {
	if f.err != nil {
		return nil, f.err
	}
	var matched []reference.Topic
	for _, topic := range f.topics {
		if filter.Level != "" && topic.Level != filter.Level {
			continue
		}
		if filter.Slug != "" && topic.Slug != filter.Slug {
			continue
		}
		matched = append(matched, topic)
	}
	return matched, nil
}

func (f *fakeReader) VocabByIDs(_ context.Context, _ reference.Language, ids []string) ([]reference.VocabEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	entries := make([]reference.VocabEntry, 0, len(ids))
	for _, id := range ids {
		if entry, ok := f.byID[id]; ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

type glossaryCall struct {
	owner    string
	language string
	lessonID string
	refs     outbound.GlossaryRefs
}

type fakeGlossary struct {
	added   []glossaryCall
	removed []glossaryCall
	entries []outbound.GlossaryEntry
	itemIDs map[progress.ItemKind][]string
	err     error
}

func (f *fakeGlossary) AddLessonWords(_ context.Context, owner, language, lessonID string, refs outbound.GlossaryRefs, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.added = append(f.added, glossaryCall{owner: owner, language: language, lessonID: lessonID, refs: refs})
	return nil
}

func (f *fakeGlossary) RemoveLessonWords(_ context.Context, owner, language, lessonID string, refs outbound.GlossaryRefs) error {
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, glossaryCall{owner: owner, language: language, lessonID: lessonID, refs: refs})
	return nil
}

func (f *fakeGlossary) Entries(_ context.Context, _, _ string, _ progress.ItemKind) ([]outbound.GlossaryEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

func (f *fakeGlossary) GlossaryItemIDs(_ context.Context, _, _ string, kind progress.ItemKind) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.itemIDs[kind], nil
}

func newService(t *testing.T, store *fakeStore, checker *fakeChecker, reader *fakeReader) *lessons.Service {
	t.Helper()
	return newServiceWithCoverage(t, store, checker, reader, &fakeCoverage{})
}

func newServiceWithCoverage(t *testing.T, store *fakeStore, checker *fakeChecker, reader *fakeReader, coverage *fakeCoverage) *lessons.Service {
	t.Helper()
	return newServiceWithSemantic(t, store, checker, reader, coverage, &fakeSemantic{})
}

func newServiceWithSemantic(t *testing.T, store *fakeStore, checker *fakeChecker, reader *fakeReader, coverage *fakeCoverage, semantic *fakeSemantic) *lessons.Service {
	t.Helper()
	return newServiceWithGlossary(t, store, checker, reader, coverage, semantic, &fakeGlossary{})
}

func newServiceWithGlossary(t *testing.T, store *fakeStore, checker *fakeChecker, reader *fakeReader, coverage *fakeCoverage, semantic *fakeSemantic, glossary *fakeGlossary) *lessons.Service {
	t.Helper()
	svc, err := lessons.NewService(store, checker, reader, coverage, semantic, store, &fakeProgressRecorder{}, glossary)
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
				ID:   "ex-1",
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
			{
				ID:              "ex-2",
				Type:            domain.TypeCloze,
				Points:          8,
				ReferencedVocab: []string{"N4#1416220"},
				Cloze: &domain.Cloze{
					Text:   "先週の{{1}}に行きました。",
					Blanks: []domain.Blank{{Index: 1, Answer: "週末"}},
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

func TestImportUsesIdempotencyKey(t *testing.T) {
	t.Parallel()

	existing := outbound.LessonRecord{Owner: "user-1", CreatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Lesson: validLesson()}
	store := &fakeStore{saveErr: domain.ErrAlreadyExists, record: existing}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	result, err := svc.Import(context.Background(), inbound.LessonImportCommand{Owner: "user-1", IdempotencyKey: "stable-request-key", Lesson: validLesson()})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Created || !result.Stored.CreatedAt.Equal(existing.CreatedAt) {
		t.Errorf("result = %+v", result)
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
	if validation.Issues[0].Path != "exercises[1].referencedVocab[0]" {
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

func TestImportAddsLessonWordsToGlossary(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	glossary := &fakeGlossary{}
	svc := newServiceWithGlossary(t, store, &fakeChecker{}, &fakeReader{}, &fakeCoverage{}, &fakeSemantic{}, glossary)
	if _, err := svc.Import(context.Background(), inbound.LessonImportCommand{Owner: "user-1", Lesson: validLesson()}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(glossary.added) != 1 {
		t.Fatalf("glossary adds = %+v, want 1", glossary.added)
	}
	call := glossary.added[0]
	if call.owner != "user-1" || call.language != "ja" || call.lessonID != validLesson().ID {
		t.Errorf("glossary call = %+v", call)
	}
	if len(call.refs.VocabIDs) != 1 || call.refs.VocabIDs[0] != "N4#1416220" || len(call.refs.GrammarIDs) != 0 {
		t.Errorf("glossary refs = %+v", call.refs)
	}
}

func TestImportWithIdempotencyKeyAddsLessonWordsToGlossary(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	glossary := &fakeGlossary{}
	svc := newServiceWithGlossary(t, store, &fakeChecker{}, &fakeReader{}, &fakeCoverage{}, &fakeSemantic{}, glossary)
	if _, err := svc.Import(context.Background(), inbound.LessonImportCommand{Owner: "user-1", IdempotencyKey: "stable-request-key", Lesson: validLesson()}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(glossary.added) != 1 || glossary.added[0].lessonID != validLesson().ID {
		t.Fatalf("glossary adds = %+v, want 1 for the imported lesson", glossary.added)
	}
}

func TestDeleteRemovesLessonWordsFromGlossary(t *testing.T) {
	t.Parallel()

	store := &fakeStore{record: outbound.LessonRecord{Owner: "user-1", Lesson: validLesson()}}
	glossary := &fakeGlossary{}
	svc := newServiceWithGlossary(t, store, &fakeChecker{}, &fakeReader{}, &fakeCoverage{}, &fakeSemantic{}, glossary)
	if err := svc.Delete(context.Background(), inbound.LessonQuery{Owner: "user-1", ID: validLesson().ID}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v", store.deleted)
	}
	if len(glossary.removed) != 1 {
		t.Fatalf("glossary removals = %+v, want 1", glossary.removed)
	}
	call := glossary.removed[0]
	if call.owner != "user-1" || call.language != "ja" || call.lessonID != validLesson().ID {
		t.Errorf("glossary call = %+v", call)
	}
	if len(call.refs.VocabIDs) != 1 || call.refs.VocabIDs[0] != "N4#1416220" {
		t.Errorf("glossary refs = %+v", call.refs)
	}
}

func TestDeleteMissingLessonLeavesGlossaryAlone(t *testing.T) {
	t.Parallel()

	store := &fakeStore{getErr: domain.ErrNotFound}
	glossary := &fakeGlossary{}
	svc := newServiceWithGlossary(t, store, &fakeChecker{}, &fakeReader{}, &fakeCoverage{}, &fakeSemantic{}, glossary)
	err := svc.Delete(context.Background(), inbound.LessonQuery{Owner: "user-1", ID: validLesson().ID})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if len(glossary.removed) != 0 || len(store.deleted) != 0 {
		t.Fatalf("removed = %+v, deleted = %v, want no mutations", glossary.removed, store.deleted)
	}
}

func TestGlossaryListsVocabHydratedFromReferenceData(t *testing.T) {
	t.Parallel()

	glossary := &fakeGlossary{entries: []outbound.GlossaryEntry{
		{ID: "N4#1416220", Language: "ja", Kind: progress.KindVocab, LessonCount: 2, AddedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "N4#1311125", Language: "ja", Kind: progress.KindVocab, LessonCount: 1, AddedAt: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)},
	}}
	reader := &fakeReader{byID: map[string]reference.VocabEntry{
		"N4#1416220": {ID: "N4#1416220", Headword: "週末", Reading: "しゅうまつ", Gloss: []string{"weekend"}, Level: "N4"},
		"N4#1311125": {ID: "N4#1311125", Headword: "写真", Reading: "しゃしん", Gloss: []string{"photograph"}, Level: "N4"},
	}}
	svc := newServiceWithGlossary(t, &fakeStore{}, &fakeChecker{}, reader, &fakeCoverage{}, &fakeSemantic{}, glossary)
	result, err := svc.Glossary(context.Background(), inbound.GlossaryQuery{Owner: "user-1", Language: "ja"})
	if err != nil {
		t.Fatalf("Glossary: %v", err)
	}
	if len(result.Languages) != 1 || result.Languages[0].Language != "ja" {
		t.Fatalf("languages = %+v", result.Languages)
	}
	words := result.Languages[0].Words
	if len(words) != 2 {
		t.Fatalf("words = %+v, want 2", words)
	}
	if words[0].ID != "N4#1311125" || words[1].ID != "N4#1416220" {
		t.Errorf("order = %s, %s, want newest first", words[0].ID, words[1].ID)
	}
	if words[1].Headword != "週末" || words[1].Reading != "しゅうまつ" || words[1].Gloss[0] != "weekend" || words[1].LessonCount != 2 {
		t.Errorf("word = %+v", words[1])
	}
}

func TestGlossaryRequiresOwnerAndKnownLanguage(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	if _, err := svc.Glossary(context.Background(), inbound.GlossaryQuery{Language: "ja"}); !errors.Is(err, domain.ErrInvalidOwner) {
		t.Fatalf("error = %v, want ErrInvalidOwner", err)
	}
	var validation *domain.ValidationError
	if _, err := svc.Glossary(context.Background(), inbound.GlossaryQuery{Owner: "user-1", Language: "xx"}); !errors.As(err, &validation) {
		t.Fatalf("error = %v, want validation error", err)
	}
}

func TestGlossaryWithoutLanguageCoversAllLanguages(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	result, err := svc.Glossary(context.Background(), inbound.GlossaryQuery{Owner: "user-1"})
	if err != nil {
		t.Fatalf("Glossary: %v", err)
	}
	if len(result.Languages) != len(domain.Languages()) {
		t.Fatalf("languages = %+v, want one group per supported language", result.Languages)
	}
}

func TestBuildPromptTreatsGlossaryWordsAsCovered(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		vocab: outbound.VocabPage{Entries: []reference.VocabEntry{
			{ID: "N4#1416220", Headword: "週末", Reading: "しゅうまつ", Gloss: []string{"weekend"}},
			{ID: "N4#1311125", Headword: "写真", Reading: "しゃしん", Gloss: []string{"photograph"}},
		}},
	}
	glossary := &fakeGlossary{itemIDs: map[progress.ItemKind][]string{
		progress.KindVocab: {"N4#1416220"},
	}}
	svc := newServiceWithGlossary(t, &fakeStore{}, &fakeChecker{}, reader, &fakeCoverage{}, &fakeSemantic{}, glossary)
	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Owner:            "user-1",
		Language:         "ja",
		Level:            "N4",
		ExerciseTypes:    []string{"cloze"},
		IncludeReference: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	covered := strings.Index(result.Prompt, "N4#1416220")
	uncovered := strings.Index(result.Prompt, "N4#1311125")
	if covered == -1 || uncovered == -1 {
		t.Fatalf("prompt is missing reference entries: covered=%d uncovered=%d", covered, uncovered)
	}
	if uncovered > covered {
		t.Errorf("glossary-covered word listed before uncovered word")
	}
}

func TestRecordSavesValidatedPerUserResult(t *testing.T) {
	t.Parallel()

	store := &fakeStore{record: outbound.LessonRecord{Lesson: validLesson()}}
	progressRecorder := &fakeProgressRecorder{}
	svc, err := lessons.NewService(store, &fakeChecker{}, &fakeReader{}, &fakeCoverage{}, &fakeSemantic{}, store, progressRecorder, &fakeGlossary{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
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
			{ExerciseID: "ex-1", Type: domain.TypeReading, Grading: "auto", Score: 0, MaxScore: 0, Correct: 1, Total: 1},
			{ExerciseID: "ex-2", Type: domain.TypeCloze, Grading: "auto", Score: 8, MaxScore: 8, Correct: 1, Total: 1},
		},
	}

	completedOn := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	if _, err := svc.Record(context.Background(), inbound.LessonResultCommand{Owner: "user-1", CompletedOn: completedOn, Result: result}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(store.results) != 1 || store.results[0].Owner != "user-1" {
		t.Fatalf("results = %+v", store.results)
	}
	if progressRecorder.owner != "user-1" || progressRecorder.result.AttemptID != result.AttemptID {
		t.Fatalf("progress = %+v", progressRecorder)
	}
	if !progressRecorder.completedOn.Equal(completedOn) {
		t.Fatalf("completedOn = %v", progressRecorder.completedOn)
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
		"wordBank",
		"multiple-choice comprehension questions",
		`comprehension questions must use "kind": "multiple_choice"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildPromptInstructionLanguageFollowsLevel(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	cases := []struct {
		language string
		level    string
		english  bool
	}{
		{"ja", "N5", true},
		{"ja", "N4", true},
		{"ja", "N3", false},
		{"ja", "N1", false},
		{"pl", "A1", true},
		{"pl", "A2", true},
		{"pl", "B1", false},
		{"my", "C2", false},
	}
	for _, tc := range cases {
		result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
			Language:      tc.language,
			Level:         tc.level,
			ExerciseTypes: []string{"cloze"},
		})
		if err != nil {
			t.Fatalf("Build(%s %s): %v", tc.language, tc.level, err)
		}
		hasEnglish := strings.Contains(result.Prompt, `"guidance" in English`)
		if hasEnglish != tc.english {
			t.Errorf("%s %s: English instructions directive = %v, want %v", tc.language, tc.level, hasEnglish, tc.english)
		}
		hasTarget := strings.Contains(result.Prompt, "phrased simply enough for a")
		if hasTarget == tc.english {
			t.Errorf("%s %s: target-language instructions directive = %v, want %v", tc.language, tc.level, hasTarget, !tc.english)
		}
	}
}

func TestBuildPromptDescribesOnlySelectedExerciseTypes(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:      "ja",
		Level:         "N4",
		ExerciseTypes: []string{"cloze"},
		ReadingStage:  "connected",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	prompt := result.Prompt
	// The selection is cloze; reading is auto-added for the connected-stage story.
	for _, want := range []string{
		"Use only these exercise types",
		"- cloze: {",
		"- reading: {",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// No shape for an unselected type may leak in and invite the model to use it.
	for _, absent := range []string{
		"- translation: {",
		"- ordering: {",
		"- matching: {",
		"- multiple_choice: {",
		"- writing_prompt:",
		"- script_practice:",
	} {
		if strings.Contains(prompt, absent) {
			t.Errorf("prompt describes unselected exercise type %q", absent)
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

func TestBuildPolishPromptIncludesStoryCoverageAndOrthography(t *testing.T) {
	t.Parallel()

	svc := newService(t, &fakeStore{}, &fakeChecker{}, &fakeReader{})
	result, err := svc.Build(context.Background(), inbound.LessonPromptQuery{
		Language:      "pl",
		Level:         "B1",
		ExerciseTypes: []string{"script_practice"},
		ReadingStage:  "connected",
		Length:        "standard",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, expected := range []string{
		"Language: Polish (pl)",
		"Level: CEFR B1",
		"idiomatic contemporary Polish",
		"85% of content-word occurrences",
		"\"kind\": \"choice\"",
		"ó/u, rz/ż, ch/h",
		"Preserve Polish diacritics exactly",
	} {
		if !strings.Contains(result.Prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
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

func TestListIncludesCompletionSummaries(t *testing.T) {
	t.Parallel()

	l := validLesson()
	store := &fakeStore{
		page: outbound.LessonPage{Records: []outbound.LessonRecord{{Owner: "user-1", Lesson: l}}},
		completions: []outbound.Completion{
			{AttemptID: "a-1", LessonID: l.ID, CompletedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC), Score: 5, MaxScore: 8},
			{AttemptID: "a-2", LessonID: l.ID, CompletedAt: time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC), Score: 7, MaxScore: 8},
		},
	}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	result, err := svc.List(context.Background(), inbound.LessonListQuery{Owner: "user-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	summary, ok := result.Completions[l.ID]
	if !ok {
		t.Fatalf("Completions = %v, want entry for %s", result.Completions, l.ID)
	}
	if summary.Count != 2 || summary.LastScore != 7 || summary.LastMaxScore != 8 {
		t.Errorf("summary = %+v", summary)
	}
	if !summary.LastCompletedAt.Equal(time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("LastCompletedAt = %v", summary.LastCompletedAt)
	}
}

func TestListWithoutCompletionsHasNoSummaries(t *testing.T) {
	t.Parallel()

	store := &fakeStore{page: outbound.LessonPage{Records: []outbound.LessonRecord{{Owner: "user-1", Lesson: validLesson()}}}}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	result, err := svc.List(context.Background(), inbound.LessonListQuery{Owner: "user-1"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Completions) != 0 {
		t.Errorf("Completions = %v, want none", result.Completions)
	}
}

func TestCompletionsReturnsPerLessonHistory(t *testing.T) {
	t.Parallel()

	l := validLesson()
	store := &fakeStore{
		record: outbound.LessonRecord{Owner: "user-1", Lesson: l},
		completions: []outbound.Completion{
			{AttemptID: "a-2", LessonID: l.ID, CompletedAt: time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC), Score: 7, MaxScore: 8},
			{AttemptID: "a-1", LessonID: l.ID, CompletedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC), Score: 5, MaxScore: 8},
			{AttemptID: "b-1", LessonID: "ffffffff-ffff-4fff-8fff-ffffffffffff", CompletedAt: time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC), Score: 3, MaxScore: 4},
		},
	}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})
	result, err := svc.Completions(context.Background(), inbound.LessonCompletionsQuery{Owner: "user-1", ID: l.ID})
	if err != nil {
		t.Fatalf("Completions: %v", err)
	}
	if len(result.Completions) != 2 {
		t.Fatalf("completions = %+v, want 2", result.Completions)
	}
	if result.Completions[0].AttemptID != "a-2" || result.Completions[0].Score != 7 {
		t.Errorf("first completion = %+v", result.Completions[0])
	}
}

func TestCompletionsClampsLimit(t *testing.T) {
	t.Parallel()

	l := validLesson()
	var completions []outbound.Completion
	for range 60 {
		completions = append(completions, outbound.Completion{LessonID: l.ID, CompletedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)})
	}
	store := &fakeStore{record: outbound.LessonRecord{Owner: "user-1", Lesson: l}, completions: completions}
	svc := newService(t, store, &fakeChecker{}, &fakeReader{})

	result, err := svc.Completions(context.Background(), inbound.LessonCompletionsQuery{Owner: "user-1", ID: l.ID})
	if err != nil {
		t.Fatalf("Completions: %v", err)
	}
	if len(result.Completions) != 10 {
		t.Errorf("default limit returned %d, want 10", len(result.Completions))
	}

	result, err = svc.Completions(context.Background(), inbound.LessonCompletionsQuery{Owner: "user-1", ID: l.ID, Limit: 200})
	if err != nil {
		t.Fatalf("Completions: %v", err)
	}
	if len(result.Completions) != 50 {
		t.Errorf("capped limit returned %d, want 50", len(result.Completions))
	}
}

func TestCompletionsValidatesQuery(t *testing.T) {
	t.Parallel()

	l := validLesson()
	svc := newService(t, &fakeStore{record: outbound.LessonRecord{Owner: "user-1", Lesson: l}}, &fakeChecker{}, &fakeReader{})
	if _, err := svc.Completions(context.Background(), inbound.LessonCompletionsQuery{ID: l.ID}); !errors.Is(err, domain.ErrInvalidOwner) {
		t.Errorf("missing owner error = %v, want ErrInvalidOwner", err)
	}
	if _, err := svc.Completions(context.Background(), inbound.LessonCompletionsQuery{Owner: "user-1", ID: "nope"}); !errors.Is(err, domain.ErrInvalidLessonID) {
		t.Errorf("bad id error = %v, want ErrInvalidLessonID", err)
	}

	missing := newService(t, &fakeStore{getErr: domain.ErrNotFound}, &fakeChecker{}, &fakeReader{})
	if _, err := missing.Completions(context.Background(), inbound.LessonCompletionsQuery{Owner: "user-1", ID: l.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing lesson error = %v, want ErrNotFound", err)
	}
}
