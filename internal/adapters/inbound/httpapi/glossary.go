package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

type glossaryWordDTO struct {
	ItemID      string   `json:"itemId"`
	Headword    string   `json:"headword"`
	Reading     string   `json:"reading,omitempty"`
	Gloss       []string `json:"gloss"`
	Level       string   `json:"level,omitempty"`
	LessonCount int      `json:"lessonCount"`
	AddedAt     string   `json:"addedAt"`
}

type glossaryLanguageDTO struct {
	Language string            `json:"language"`
	Words    []glossaryWordDTO `json:"words"`
}

func (h *Handler) handleGlossary(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	result, err := h.glossary.Glossary(ctx, inbound.GlossaryQuery{
		Owner:    owner,
		Language: req.QueryStringParameters["language"],
	})
	if err != nil {
		return lessonError(ctx, err)
	}
	languages := make([]glossaryLanguageDTO, 0, len(result.Languages))
	for _, group := range result.Languages {
		words := make([]glossaryWordDTO, 0, len(group.Words))
		for _, word := range group.Words {
			gloss := word.Gloss
			if gloss == nil {
				gloss = []string{}
			}
			words = append(words, glossaryWordDTO{
				ItemID:      word.ID,
				Headword:    word.Headword,
				Reading:     word.Reading,
				Gloss:       gloss,
				Level:       word.Level,
				LessonCount: word.LessonCount,
				AddedAt:     word.AddedAt.UTC().Format(time.RFC3339),
			})
		}
		languages = append(languages, glossaryLanguageDTO{Language: group.Language, Words: words})
	}
	return respondJSON(http.StatusOK, map[string]any{"languages": languages})
}
