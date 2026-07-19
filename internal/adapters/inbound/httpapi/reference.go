package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/aws/aws-lambda-go/events"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

func (h *Handler) handleVocab(ctx context.Context, params map[string]string) events.APIGatewayV2HTTPResponse {
	limit, ok := parseLimit(params["limit"])
	if !ok {
		return errorJSON(http.StatusBadRequest, "limit must be a positive integer")
	}
	result, err := h.reference.Vocab(ctx, inbound.VocabQuery{
		Language: params["lang"],
		Level:    params["level"],
		Topic:    params["topic"],
		Limit:    limit,
		Cursor:   params["cursor"],
	})
	if err != nil {
		return referenceError(ctx, err)
	}

	items := make([]vocabEntryDTO, 0, len(result.Entries))
	for _, entry := range result.Entries {
		items = append(items, vocabEntryDTO{
			ID:               entry.ID,
			Headword:         entry.Headword,
			Reading:          entry.Reading,
			Gloss:            entry.Gloss,
			Pos:              entry.PartsOfSpeech,
			Level:            string(entry.Level),
			LevelApproximate: entry.LevelApproximate,
			FreqBand:         entry.FreqBand,
			Topics:           entry.Topics,
			Example:          toExampleDTO(entry.Example),
			SourceID:         entry.SourceID,
			License:          entry.License,
		})
	}
	return respondJSON(http.StatusOK, pageResponse[vocabEntryDTO]{Items: items, NextCursor: result.NextCursor})
}

func (h *Handler) handleGrammar(ctx context.Context, params map[string]string) events.APIGatewayV2HTTPResponse {
	limit, ok := parseLimit(params["limit"])
	if !ok {
		return errorJSON(http.StatusBadRequest, "limit must be a positive integer")
	}
	result, err := h.reference.Grammar(ctx, inbound.GrammarQuery{
		Language: params["lang"],
		Level:    params["level"],
		Limit:    limit,
		Cursor:   params["cursor"],
	})
	if err != nil {
		return referenceError(ctx, err)
	}

	items := make([]grammarTopicDTO, 0, len(result.Topics))
	for _, topic := range result.Topics {
		items = append(items, grammarTopicDTO{
			ID:          topic.ID,
			TopicID:     topic.TopicID,
			Name:        topic.Name,
			Level:       string(topic.Level),
			Description: topic.Description,
			Example:     toExampleDTO(topic.Example),
			SourceID:    topic.SourceID,
			License:     topic.License,
		})
	}
	return respondJSON(http.StatusOK, pageResponse[grammarTopicDTO]{Items: items, NextCursor: result.NextCursor})
}

func (h *Handler) handleScripts(ctx context.Context, params map[string]string) events.APIGatewayV2HTTPResponse {
	limit, ok := parseLimit(params["limit"])
	if !ok {
		return errorJSON(http.StatusBadRequest, "limit must be a positive integer")
	}
	result, err := h.reference.Scripts(ctx, inbound.ScriptQuery{
		Language:   params["lang"],
		ScriptType: params["type"],
		Level:      params["level"],
		Limit:      limit,
		Cursor:     params["cursor"],
	})
	if err != nil {
		return referenceError(ctx, err)
	}

	items := make([]scriptGlyphDTO, 0, len(result.Glyphs))
	for _, glyph := range result.Glyphs {
		items = append(items, scriptGlyphDTO{
			Glyph:         glyph.Glyph,
			ScriptType:    string(glyph.ScriptType),
			Name:          glyph.Name,
			Meanings:      glyph.Meanings,
			Readings:      glyph.Readings,
			KanaScript:    glyph.KanaScript,
			Level:         string(glyph.Level),
			Grade:         glyph.Grade,
			StrokeCount:   glyph.StrokeCount,
			StrokeDataRef: glyph.StrokeDataRef,
			Components:    glyph.Components,
			SourceID:      glyph.SourceID,
			License:       glyph.License,
		})
	}
	return respondJSON(http.StatusOK, pageResponse[scriptGlyphDTO]{Items: items, NextCursor: result.NextCursor})
}

func (h *Handler) handleReadings(ctx context.Context, params map[string]string) events.APIGatewayV2HTTPResponse {
	limit, ok := parseLimit(params["limit"])
	if !ok {
		return errorJSON(http.StatusBadRequest, "limit must be a positive integer")
	}
	result, err := h.reference.Readings(ctx, inbound.ReadingQuery{
		Language: params["lang"], Level: params["level"], Limit: limit, Cursor: params["cursor"],
	})
	if err != nil {
		return referenceError(ctx, err)
	}
	items := make([]readingPassageDTO, 0, len(result.Passages))
	for _, passage := range result.Passages {
		items = append(items, readingPassageDTO{
			ID: passage.ID, Text: passage.Text, Level: string(passage.Level), LevelApproximate: passage.LevelApproximate,
			Coverage: passage.Coverage, SourceID: passage.SourceID, License: passage.License,
		})
	}
	return respondJSON(http.StatusOK, pageResponse[readingPassageDTO]{Items: items, NextCursor: result.NextCursor})
}

type pageResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type exampleDTO struct {
	Text        string `json:"text"`
	Translation string `json:"translation"`
	SourceID    string `json:"sourceId,omitempty"`
	License     string `json:"license,omitempty"`
}

func toExampleDTO(example *domain.Example) *exampleDTO {
	if example == nil {
		return nil
	}
	return &exampleDTO{
		Text:        example.Text,
		Translation: example.Translation,
		SourceID:    example.SourceID,
		License:     example.License,
	}
}

type vocabEntryDTO struct {
	ID               string      `json:"id"`
	Headword         string      `json:"headword"`
	Reading          string      `json:"reading"`
	Gloss            []string    `json:"gloss"`
	Pos              []string    `json:"pos"`
	Level            string      `json:"level"`
	LevelApproximate bool        `json:"levelApproximate,omitempty"`
	FreqBand         int         `json:"freqBand,omitempty"`
	Topics           []string    `json:"topics,omitempty"`
	Example          *exampleDTO `json:"example,omitempty"`
	SourceID         string      `json:"sourceId"`
	License          string      `json:"license"`
}

type grammarTopicDTO struct {
	ID          string      `json:"id"`
	TopicID     string      `json:"topicId"`
	Name        string      `json:"name"`
	Level       string      `json:"level"`
	Description string      `json:"description"`
	Example     *exampleDTO `json:"example,omitempty"`
	SourceID    string      `json:"sourceId"`
	License     string      `json:"license"`
}

type scriptGlyphDTO struct {
	Glyph         string              `json:"glyph"`
	ScriptType    string              `json:"scriptType"`
	Name          string              `json:"name"`
	Meanings      []string            `json:"meanings,omitempty"`
	Readings      map[string][]string `json:"readings"`
	KanaScript    string              `json:"kanaScript,omitempty"`
	Level         string              `json:"level,omitempty"`
	Grade         int                 `json:"grade,omitempty"`
	StrokeCount   int                 `json:"strokeCount,omitempty"`
	StrokeDataRef string              `json:"strokeDataRef,omitempty"`
	Components    []string            `json:"components,omitempty"`
	SourceID      string              `json:"sourceId"`
	License       string              `json:"license"`
}

type readingPassageDTO struct {
	ID               string  `json:"id"`
	Text             string  `json:"text"`
	Level            string  `json:"level"`
	LevelApproximate bool    `json:"levelApproximate"`
	Coverage         float64 `json:"coverage"`
	SourceID         string  `json:"sourceId"`
	License          string  `json:"license"`
}

var badRequestErrors = []error{
	domain.ErrInvalidLanguage,
	domain.ErrInvalidLevel,
	domain.ErrInvalidScriptType,
	domain.ErrInvalidTopic,
	domain.ErrInvalidCursor,
	domain.ErrLevelWithoutType,
}

func referenceError(ctx context.Context, err error) events.APIGatewayV2HTTPResponse {
	for _, sentinel := range badRequestErrors {
		if errors.Is(err, sentinel) {
			return errorJSON(http.StatusBadRequest, sentinel.Error())
		}
	}
	slog.ErrorContext(ctx, "reference query failed", "error", err)
	return errorJSON(http.StatusInternalServerError, "internal error")
}

func parseLimit(raw string) (int, bool) {
	if raw == "" {
		return 0, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, false
	}
	return limit, true
}
