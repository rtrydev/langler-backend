package inbound

import (
	"context"

	"github.com/rtrydev/langler-backend/internal/domain/reference"
)

type VocabQuery struct {
	Language string
	Level    string
	Topic    string
	Limit    int
	Cursor   string
}

type VocabResult struct {
	Entries    []reference.VocabEntry
	NextCursor string
}

type GrammarQuery struct {
	Language string
	Level    string
	Limit    int
	Cursor   string
}

type GrammarResult struct {
	Topics     []reference.GrammarTopic
	NextCursor string
}

type ScriptQuery struct {
	Language   string
	ScriptType string
	Level      string
	Limit      int
	Cursor     string
}

type ScriptResult struct {
	Glyphs     []reference.ScriptGlyph
	NextCursor string
}

type ReferenceProvider interface {
	Vocab(ctx context.Context, query VocabQuery) (VocabResult, error)
	Grammar(ctx context.Context, query GrammarQuery) (GrammarResult, error)
	Scripts(ctx context.Context, query ScriptQuery) (ScriptResult, error)
}
