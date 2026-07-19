package reference_test

import (
	"context"
	"errors"
	"testing"

	appref "github.com/rtrydev/langler-backend/internal/application/reference"
	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
	"github.com/rtrydev/langler-backend/internal/ports/outbound"
)

type fakeReader struct {
	vocabFilter   outbound.VocabFilter
	grammarFilter outbound.GrammarFilter
	scriptFilter  outbound.ScriptFilter
	readingFilter outbound.ReadingFilter
	vocabPage     outbound.VocabPage
	grammarPage   outbound.GrammarPage
	scriptPage    outbound.ScriptPage
	readingPage   outbound.ReadingPage
	err           error
}

func (f *fakeReader) Vocab(_ context.Context, filter outbound.VocabFilter) (outbound.VocabPage, error) {
	f.vocabFilter = filter
	return f.vocabPage, f.err
}

func (f *fakeReader) Grammar(_ context.Context, filter outbound.GrammarFilter) (outbound.GrammarPage, error) {
	f.grammarFilter = filter
	return f.grammarPage, f.err
}

func (f *fakeReader) Scripts(_ context.Context, filter outbound.ScriptFilter) (outbound.ScriptPage, error) {
	f.scriptFilter = filter
	return f.scriptPage, f.err
}

func (f *fakeReader) Readings(_ context.Context, filter outbound.ReadingFilter) (outbound.ReadingPage, error) {
	f.readingFilter = filter
	return f.readingPage, f.err
}

func (f *fakeReader) Topics(_ context.Context, _ outbound.TopicFilter) ([]domain.Topic, error) {
	return nil, f.err
}

func (f *fakeReader) VocabByIDs(_ context.Context, _ domain.Language, _ []string) ([]domain.VocabEntry, error) {
	return nil, f.err
}

func TestNewServiceRejectsNilReader(t *testing.T) {
	t.Parallel()

	if _, err := appref.NewService(nil); err == nil {
		t.Fatal("NewService(nil) error = nil")
	}
}

func TestVocabValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      inbound.VocabQuery
		wantFilter outbound.VocabFilter
		wantErr    error
	}{
		{
			name:  "defaults applied",
			query: inbound.VocabQuery{Language: "ja"},
			wantFilter: outbound.VocabFilter{
				Language: "ja",
				Limit:    50,
			},
		},
		{
			name:  "level normalized and topic passed",
			query: inbound.VocabQuery{Language: "ja", Level: "n5", Topic: "daily-life", Limit: 10, Cursor: "abc"},
			wantFilter: outbound.VocabFilter{
				Language: "ja",
				Level:    "N5",
				Topic:    "daily-life",
				Limit:    10,
				Cursor:   "abc",
			},
		},
		{
			name:  "limit capped",
			query: inbound.VocabQuery{Language: "ja", Limit: 5000},
			wantFilter: outbound.VocabFilter{
				Language: "ja",
				Limit:    200,
			},
		},
		{
			name:    "invalid language",
			query:   inbound.VocabQuery{Language: "Japanese"},
			wantErr: domain.ErrInvalidLanguage,
		},
		{
			name:    "invalid level",
			query:   inbound.VocabQuery{Language: "ja", Level: "N5#oops"},
			wantErr: domain.ErrInvalidLevel,
		},
		{
			name:    "invalid topic",
			query:   inbound.VocabQuery{Language: "ja", Topic: "Daily Life"},
			wantErr: domain.ErrInvalidTopic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reader := &fakeReader{vocabPage: outbound.VocabPage{NextCursor: "next"}}
			svc, err := appref.NewService(reader)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			result, err := svc.Vocab(context.Background(), tt.query)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Vocab error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if reader.vocabFilter != tt.wantFilter {
				t.Errorf("filter = %+v, want %+v", reader.vocabFilter, tt.wantFilter)
			}
			if result.NextCursor != "next" {
				t.Errorf("NextCursor = %q, want %q", result.NextCursor, "next")
			}
		})
	}
}

func TestGrammarValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{grammarPage: outbound.GrammarPage{
		Topics: []domain.GrammarTopic{{TopicID: "particle-wa"}},
	}}
	svc, err := appref.NewService(reader)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Grammar(context.Background(), inbound.GrammarQuery{Language: "ja", Level: "n4"})
	if err != nil {
		t.Fatalf("Grammar: %v", err)
	}
	want := outbound.GrammarFilter{Language: "ja", Level: "N4", Limit: 50}
	if reader.grammarFilter != want {
		t.Errorf("filter = %+v, want %+v", reader.grammarFilter, want)
	}
	if len(result.Topics) != 1 || result.Topics[0].TopicID != "particle-wa" {
		t.Errorf("Topics = %+v, want the fake topic", result.Topics)
	}

	if _, err := svc.Grammar(context.Background(), inbound.GrammarQuery{Language: "j"}); !errors.Is(err, domain.ErrInvalidLanguage) {
		t.Fatalf("Grammar invalid language error = %v, want %v", err, domain.ErrInvalidLanguage)
	}
}

func TestScriptsValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{scriptPage: outbound.ScriptPage{
		Glyphs: []domain.ScriptGlyph{{Glyph: "あ"}},
	}}
	svc, err := appref.NewService(reader)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Scripts(context.Background(), inbound.ScriptQuery{Language: "ja", ScriptType: "kanji", Level: "N2"})
	if err != nil {
		t.Fatalf("Scripts: %v", err)
	}
	want := outbound.ScriptFilter{Language: "ja", ScriptType: "kanji", Level: "N2", Limit: 50}
	if reader.scriptFilter != want {
		t.Errorf("filter = %+v, want %+v", reader.scriptFilter, want)
	}
	if len(result.Glyphs) != 1 || result.Glyphs[0].Glyph != "あ" {
		t.Errorf("Glyphs = %+v, want the fake glyph", result.Glyphs)
	}

	if _, err := svc.Scripts(context.Background(), inbound.ScriptQuery{Language: "ja", ScriptType: "Kanji"}); !errors.Is(err, domain.ErrInvalidScriptType) {
		t.Fatalf("Scripts invalid type error = %v, want %v", err, domain.ErrInvalidScriptType)
	}
}

func TestReadingsValidatesAndDelegates(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{readingPage: outbound.ReadingPage{
		Passages:   []domain.ReadingPassage{{ID: "story-1", Level: "A2"}},
		NextCursor: "next",
	}}
	svc, err := appref.NewService(reader)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Readings(context.Background(), inbound.ReadingQuery{Language: "my", Level: "a2", Limit: 25, Cursor: "current"})
	if err != nil {
		t.Fatalf("Readings: %v", err)
	}
	want := outbound.ReadingFilter{Language: "my", Level: "A2", Limit: 25, Cursor: "current"}
	if reader.readingFilter != want {
		t.Errorf("filter = %+v, want %+v", reader.readingFilter, want)
	}
	if len(result.Passages) != 1 || result.Passages[0].ID != "story-1" || result.NextCursor != "next" {
		t.Errorf("result = %+v, want fake passage and cursor", result)
	}
}

func TestReaderErrorsPropagate(t *testing.T) {
	t.Parallel()

	readerErr := errors.New("storage down")
	svc, err := appref.NewService(&fakeReader{err: readerErr})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.Vocab(context.Background(), inbound.VocabQuery{Language: "ja"}); !errors.Is(err, readerErr) {
		t.Fatalf("Vocab error = %v, want %v", err, readerErr)
	}
}
