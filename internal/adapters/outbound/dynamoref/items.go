package dynamoref

import (
	"strings"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
)

type exampleItem struct {
	Text        string `dynamodbav:"text"`
	Translation string `dynamodbav:"translation"`
	SourceID    string `dynamodbav:"sourceId"`
	License     string `dynamodbav:"license"`
}

func (e *exampleItem) toDomain() *domain.Example {
	if e == nil {
		return nil
	}
	return &domain.Example{
		Text:        e.Text,
		Translation: e.Translation,
		SourceID:    e.SourceID,
		License:     e.License,
	}
}

type vocabItem struct {
	SK       string       `dynamodbav:"SK"`
	Headword string       `dynamodbav:"headword"`
	Reading  string       `dynamodbav:"reading"`
	Gloss    []string     `dynamodbav:"gloss"`
	Pos      []string     `dynamodbav:"pos"`
	Level    string       `dynamodbav:"level"`
	FreqBand int          `dynamodbav:"freqBand"`
	Topics   []string     `dynamodbav:"topics"`
	Example  *exampleItem `dynamodbav:"example"`
	SourceID string       `dynamodbav:"sourceId"`
	License  string       `dynamodbav:"license"`
}

func (v vocabItem) toDomain() domain.VocabEntry {
	return domain.VocabEntry{
		ID:            strings.TrimPrefix(v.SK, "VOCAB#"),
		Headword:      v.Headword,
		Reading:       v.Reading,
		Gloss:         v.Gloss,
		PartsOfSpeech: v.Pos,
		Level:         domain.Level(v.Level),
		FreqBand:      v.FreqBand,
		Topics:        v.Topics,
		Example:       v.Example.toDomain(),
		SourceID:      v.SourceID,
		License:       v.License,
	}
}

type topicItem struct {
	Slug        string   `dynamodbav:"slug"`
	Name        string   `dynamodbav:"name"`
	Description string   `dynamodbav:"description"`
	Level       string   `dynamodbav:"level"`
	Keywords    []string `dynamodbav:"keywords"`
	VocabIDs    []string `dynamodbav:"vocabIds"`
}

func (t topicItem) toDomain() domain.Topic {
	return domain.Topic{
		Slug:        domain.TopicTag(t.Slug),
		Name:        t.Name,
		Description: t.Description,
		Level:       domain.Level(t.Level),
		Keywords:    t.Keywords,
		VocabIDs:    t.VocabIDs,
	}
}

type grammarItem struct {
	SK          string       `dynamodbav:"SK"`
	TopicID     string       `dynamodbav:"topicId"`
	Name        string       `dynamodbav:"name"`
	Level       string       `dynamodbav:"level"`
	Description string       `dynamodbav:"description"`
	Example     *exampleItem `dynamodbav:"example"`
	SourceID    string       `dynamodbav:"sourceId"`
	License     string       `dynamodbav:"license"`
}

func (g grammarItem) toDomain() domain.GrammarTopic {
	return domain.GrammarTopic{
		ID:          strings.TrimPrefix(g.SK, "GRAMMAR#"),
		TopicID:     g.TopicID,
		Name:        g.Name,
		Level:       domain.Level(g.Level),
		Description: g.Description,
		Example:     g.Example.toDomain(),
		SourceID:    g.SourceID,
		License:     g.License,
	}
}

type scriptItem struct {
	Glyph         string              `dynamodbav:"glyph"`
	ScriptType    string              `dynamodbav:"scriptType"`
	Name          string              `dynamodbav:"name"`
	Meanings      []string            `dynamodbav:"meanings"`
	Readings      map[string][]string `dynamodbav:"readings"`
	KanaScript    string              `dynamodbav:"kanaScript"`
	Level         string              `dynamodbav:"level"`
	Grade         int                 `dynamodbav:"grade"`
	StrokeCount   int                 `dynamodbav:"strokeCount"`
	StrokeDataRef string              `dynamodbav:"strokeDataRef"`
	Components    []string            `dynamodbav:"components"`
	SourceID      string              `dynamodbav:"sourceId"`
	License       string              `dynamodbav:"license"`
}

func (s scriptItem) toDomain() domain.ScriptGlyph {
	return domain.ScriptGlyph{
		Glyph:         s.Glyph,
		ScriptType:    domain.ScriptType(s.ScriptType),
		Name:          s.Name,
		Meanings:      s.Meanings,
		Readings:      s.Readings,
		KanaScript:    s.KanaScript,
		Level:         domain.Level(s.Level),
		Grade:         s.Grade,
		StrokeCount:   s.StrokeCount,
		StrokeDataRef: s.StrokeDataRef,
		Components:    s.Components,
		SourceID:      s.SourceID,
		License:       s.License,
	}
}
