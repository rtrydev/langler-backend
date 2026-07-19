package assessments_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/rtrydev/langler-backend/internal/application/assessments"
	domain "github.com/rtrydev/langler-backend/internal/domain/assessment"
	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type fakeStore struct {
	sessions map[string]domain.Session
	levels   map[string]outbound.ProfileLevelRecord
	saveErr  error
	saves    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{sessions: map[string]domain.Session{}, levels: map[string]outbound.ProfileLevelRecord{}}
}

func (f *fakeStore) Create(_ context.Context, owner string, session domain.Session) error {
	f.sessions[owner+"#"+session.ID] = session
	return nil
}

func (f *fakeStore) Get(_ context.Context, owner, id string) (domain.Session, error) {
	session, ok := f.sessions[owner+"#"+id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return session, nil
}

func (f *fakeStore) Save(_ context.Context, owner string, session domain.Session, level *outbound.ProfileLevelRecord) error {
	f.saves++
	if f.saveErr != nil {
		return f.saveErr
	}
	stored, ok := f.sessions[owner+"#"+session.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if stored.Version != session.Version-1 {
		return domain.ErrConflict
	}
	f.sessions[owner+"#"+session.ID] = session
	if level != nil {
		f.levels[owner+"#"+level.Language] = *level
	}
	return nil
}

func (f *fakeStore) List(_ context.Context, owner string) ([]domain.Session, error) {
	var sessions []domain.Session
	for key, session := range f.sessions {
		if key == owner+"#"+session.ID {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func (f *fakeStore) Levels(_ context.Context, owner string) ([]outbound.ProfileLevelRecord, error) {
	var levels []outbound.ProfileLevelRecord
	for key, level := range f.levels {
		if key == owner+"#"+level.Language {
			levels = append(levels, level)
		}
	}
	return levels, nil
}

type fakeReader struct {
	vocabByLevel   map[string][]reference.VocabEntry
	grammarByLevel map[string][]reference.GrammarTopic
}

func newFakeReader(levels []string) *fakeReader {
	reader := &fakeReader{vocabByLevel: map[string][]reference.VocabEntry{}, grammarByLevel: map[string][]reference.GrammarTopic{}}
	for _, level := range levels {
		for i := range 20 {
			reader.vocabByLevel[level] = append(reader.vocabByLevel[level], reference.VocabEntry{
				ID:            fmt.Sprintf("%s#%d", level, i),
				Headword:      fmt.Sprintf("%s-word-%d", level, i),
				Reading:       fmt.Sprintf("%s-reading-%d", level, i),
				PartsOfSpeech: []string{"n"},
				Gloss:         []string{fmt.Sprintf("%s-gloss-%d", level, i)},
				Example: &reference.Example{
					Text:        fmt.Sprintf("%s-sentence-%d", level, i),
					Translation: fmt.Sprintf("%s-translation-%d", level, i),
				},
				Level: reference.Level(level),
			})
		}
		for i := range 6 {
			reader.grammarByLevel[level] = append(reader.grammarByLevel[level], reference.GrammarTopic{
				ID:    fmt.Sprintf("%s#topic-%d", level, i),
				Level: reference.Level(level),
				Example: &reference.Example{
					Text:        fmt.Sprintf("%s-grammar-sentence-%d", level, i),
					Translation: fmt.Sprintf("%s-grammar-translation-%d", level, i),
				},
			})
		}
	}
	return reader
}

func (f *fakeReader) Vocab(_ context.Context, filter outbound.VocabFilter) (outbound.VocabPage, error) {
	return outbound.VocabPage{Entries: f.vocabByLevel[string(filter.Level)]}, nil
}

func (f *fakeReader) Grammar(_ context.Context, filter outbound.GrammarFilter) (outbound.GrammarPage, error) {
	return outbound.GrammarPage{Topics: f.grammarByLevel[string(filter.Level)]}, nil
}

func (f *fakeReader) Scripts(_ context.Context, _ outbound.ScriptFilter) (outbound.ScriptPage, error) {
	return outbound.ScriptPage{}, nil
}

func newService(t *testing.T, store *fakeStore) *assessments.Service {
	t.Helper()
	service, err := assessments.NewService(store, newFakeReader([]string{"N5", "N4", "N3", "N2", "N1"}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func storedAnswers(t *testing.T, store *fakeStore, owner, id string, stageIndex int, correctly func(band string) bool) []int {
	t.Helper()
	session, err := store.Get(context.Background(), owner, id)
	if err != nil {
		t.Fatalf("stored session: %v", err)
	}
	stage := session.Stages[stageIndex]
	answers := make([]int, len(stage.Items))
	for i, item := range stage.Items {
		if correctly(stage.Band) {
			answers[i] = item.CorrectIndex
		} else {
			answers[i] = (item.CorrectIndex + 1) % len(item.Options)
		}
	}
	return answers
}

func runPlacement(t *testing.T, trueLevel string) inbound.AssessmentView {
	t.Helper()
	store := newFakeStore()
	service := newService(t, store)
	ctx := context.Background()

	view, err := service.Start(ctx, inbound.AssessmentStartCommand{Owner: "user-1", Language: "ja"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	bands := []string{"N5", "N4", "N3", "N2", "N1"}
	trueIndex := slices.Index(bands, trueLevel)
	knows := func(band string) bool {
		return slices.Index(bands, band) <= trueIndex
	}
	for view.Status == string(domain.StatusInProgress) {
		answers := storedAnswers(t, store, "user-1", view.ID, view.Stage.Index, knows)
		view, err = service.Answer(ctx, inbound.AssessmentAnswerCommand{
			Owner: "user-1", AssessmentID: view.ID, StageIndex: view.Stage.Index, Answers: answers,
		})
		if err != nil {
			t.Fatalf("Answer: %v", err)
		}
	}
	return view
}

func TestSeededAnswerPatternsTrackItemDifficulty(t *testing.T) {
	t.Parallel()

	for _, trueLevel := range []string{"N5", "N3", "N1"} {
		view := runPlacement(t, trueLevel)
		if view.Result == nil || view.Result.EstimatedLevel != trueLevel {
			t.Fatalf("true level %s estimated %+v", trueLevel, view.Result)
		}
		if view.Result.Floor {
			t.Fatalf("true level %s flagged as floor", trueLevel)
		}
	}
}

func TestStartBuildsFirstStageWithoutLeakingAnswers(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := newService(t, store)

	view, err := service.Start(context.Background(), inbound.AssessmentStartCommand{Owner: "user-1", Language: "ja"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if view.Stage == nil || view.Stage.Band != "N5" || view.Stage.BandCount != 5 || len(view.Stage.Items) != 10 {
		t.Fatalf("stage = %+v", view.Stage)
	}
	if view.Status != string(domain.StatusInProgress) {
		t.Fatalf("status = %s", view.Status)
	}

	if _, err := service.Start(context.Background(), inbound.AssessmentStartCommand{Owner: "", Language: "ja"}); !errors.Is(err, lesson.ErrInvalidOwner) {
		t.Fatalf("missing owner error = %v", err)
	}
	if _, err := service.Start(context.Background(), inbound.AssessmentStartCommand{Owner: "user-1", Language: "xx"}); !errors.Is(err, domain.ErrInvalidAssessment) {
		t.Fatalf("unknown language error = %v", err)
	}
}

func TestCompletionRecordsProfileLevel(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := newService(t, store)
	ctx := context.Background()

	view, err := service.Start(ctx, inbound.AssessmentStartCommand{Owner: "user-1", Language: "ja"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	answers := storedAnswers(t, store, "user-1", view.ID, 0, func(string) bool { return false })
	completed, err := service.Answer(ctx, inbound.AssessmentAnswerCommand{Owner: "user-1", AssessmentID: view.ID, StageIndex: 0, Answers: answers})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if completed.Result == nil || completed.Result.EstimatedLevel != "N5" || !completed.Result.Floor {
		t.Fatalf("result = %+v", completed.Result)
	}

	levels, err := service.Levels(ctx, "user-1")
	if err != nil {
		t.Fatalf("Levels: %v", err)
	}
	if len(levels) != 1 || levels[0].Language != "ja" || levels[0].Level != "N5" || levels[0].AssessmentID != view.ID {
		t.Fatalf("levels = %+v", levels)
	}

	summaries, err := service.Assessments(ctx, "user-1")
	if err != nil {
		t.Fatalf("Assessments: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Status != string(domain.StatusCompleted) || summaries[0].EstimatedLevel != "N5" {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestAnswerReplaysIdempotently(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service := newService(t, store)
	ctx := context.Background()

	view, err := service.Start(ctx, inbound.AssessmentStartCommand{Owner: "user-1", Language: "ja"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	answers := storedAnswers(t, store, "user-1", view.ID, 0, func(string) bool { return true })
	first, err := service.Answer(ctx, inbound.AssessmentAnswerCommand{Owner: "user-1", AssessmentID: view.ID, StageIndex: 0, Answers: answers})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	replay, err := service.Answer(ctx, inbound.AssessmentAnswerCommand{Owner: "user-1", AssessmentID: view.ID, StageIndex: 0, Answers: answers})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Stage == nil || first.Stage == nil || replay.Stage.Index != first.Stage.Index {
		t.Fatalf("replay = %+v, first = %+v", replay.Stage, first.Stage)
	}

	diverging := append([]int(nil), answers...)
	diverging[0] = (diverging[0] + 1) % 4
	if _, err := service.Answer(ctx, inbound.AssessmentAnswerCommand{Owner: "user-1", AssessmentID: view.ID, StageIndex: 0, Answers: diverging}); !errors.Is(err, domain.ErrInvalidAnswer) {
		t.Fatalf("diverging replay error = %v", err)
	}
}

func TestStartFailsWhenReferenceDataIsMissing(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	service, err := assessments.NewService(store, newFakeReader(nil))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := service.Start(context.Background(), inbound.AssessmentStartCommand{Owner: "user-1", Language: "pl"}); !errors.Is(err, domain.ErrInsufficientReference) {
		t.Fatalf("error = %v, want ErrInsufficientReference", err)
	}
}
