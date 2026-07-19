package reference

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalidLanguage   = errors.New("language must be a two- or three-letter lowercase code")
	ErrInvalidLevel      = errors.New("level must be one to eight uppercase letters or digits")
	ErrInvalidScriptType = errors.New("script type must be one to sixteen lowercase letters")
	ErrInvalidTopic      = errors.New("topic must be lowercase letters, digits, or hyphens")
	ErrInvalidCursor     = errors.New("cursor is not a valid pagination token")
	ErrLevelWithoutType  = errors.New("script level filter requires a script type")
	ErrStorageFailure    = errors.New("reference storage failed")
)

var (
	languagePattern   = regexp.MustCompile(`^[a-z]{2,3}$`)
	levelPattern      = regexp.MustCompile(`^[A-Z0-9]{1,8}$`)
	scriptTypePattern = regexp.MustCompile(`^[a-z]{1,16}$`)
	topicPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

type Language string

func NewLanguage(code string) (Language, error) {
	if !languagePattern.MatchString(code) {
		return "", ErrInvalidLanguage
	}
	return Language(code), nil
}

type Level string

func NewLevel(band string) (Level, error) {
	normalized := strings.ToUpper(band)
	if !levelPattern.MatchString(normalized) {
		return "", ErrInvalidLevel
	}
	return Level(normalized), nil
}

type ScriptType string

func NewScriptType(name string) (ScriptType, error) {
	if !scriptTypePattern.MatchString(name) {
		return "", ErrInvalidScriptType
	}
	return ScriptType(name), nil
}

type TopicTag string

func NewTopicTag(tag string) (TopicTag, error) {
	if !topicPattern.MatchString(tag) {
		return "", ErrInvalidTopic
	}
	return TopicTag(tag), nil
}

type Example struct {
	Text        string
	Translation string
	SourceID    string
	License     string
}

type VocabEntry struct {
	ID               string
	Headword         string
	Reading          string
	Gloss            []string
	PartsOfSpeech    []string
	Level            Level
	LevelApproximate bool
	FreqBand         int
	Topics           []string
	Example          *Example
	SourceID         string
	License          string
}

type Topic struct {
	Slug        TopicTag
	Name        string
	Description string
	Level       Level
	Keywords    []string
	VocabIDs    []string
}

type GrammarTopic struct {
	ID          string
	TopicID     string
	Name        string
	Level       Level
	Description string
	Example     *Example
	SourceID    string
	License     string
}

type ScriptGlyph struct {
	Glyph         string
	ScriptType    ScriptType
	Name          string
	Meanings      []string
	Readings      map[string][]string
	KanaScript    string
	Level         Level
	Grade         int
	StrokeCount   int
	StrokeDataRef string
	Components    []string
	SourceID      string
	License       string
}
