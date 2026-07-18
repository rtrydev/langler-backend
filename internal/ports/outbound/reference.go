package outbound

import (
	"context"

	"github.com/rtrydev/langler-backend/internal/domain/reference"
)

type VocabFilter struct {
	Language reference.Language
	Level    reference.Level
	Topic    reference.TopicTag
	Limit    int
	Cursor   string
}

type VocabPage struct {
	Entries    []reference.VocabEntry
	NextCursor string
}

type GrammarFilter struct {
	Language reference.Language
	Level    reference.Level
	Limit    int
	Cursor   string
}

type GrammarPage struct {
	Topics     []reference.GrammarTopic
	NextCursor string
}

type ScriptFilter struct {
	Language   reference.Language
	ScriptType reference.ScriptType
	Level      reference.Level
	Limit      int
	Cursor     string
}

type ScriptPage struct {
	Glyphs     []reference.ScriptGlyph
	NextCursor string
}

type ReferenceReader interface {
	Vocab(ctx context.Context, filter VocabFilter) (VocabPage, error)
	Grammar(ctx context.Context, filter GrammarFilter) (GrammarPage, error)
	Scripts(ctx context.Context, filter ScriptFilter) (ScriptPage, error)
}
