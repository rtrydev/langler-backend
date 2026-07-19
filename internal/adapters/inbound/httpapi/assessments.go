package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/rtrydev/langler-backend/internal/domain/assessment"
	"github.com/rtrydev/langler-backend/internal/domain/lesson"
	"github.com/rtrydev/langler-backend/internal/domain/reference"
	"github.com/rtrydev/langler-backend/internal/ports/inbound"
)

const assessmentGuidance = "This estimate is approximate guidance based on community-mapped reference data, not an official JLPT or CEFR certification."

type assessmentItemDTO struct {
	Kind    string   `json:"kind"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
}

type assessmentStageDTO struct {
	Index     int                 `json:"index"`
	Band      string              `json:"band"`
	BandCount int                 `json:"bandCount"`
	Items     []assessmentItemDTO `json:"items"`
}

type assessmentBandDTO struct {
	Band    string `json:"band"`
	Correct int    `json:"correct"`
	Total   int    `json:"total"`
	Passed  bool   `json:"passed"`
}

type assessmentResultDTO struct {
	EstimatedLevel string              `json:"estimatedLevel"`
	Confidence     string              `json:"confidence"`
	Floor          bool                `json:"floor"`
	Bands          []assessmentBandDTO `json:"bands"`
}

type assessmentViewDTO struct {
	AssessmentID string               `json:"assessmentId"`
	Language     string               `json:"language"`
	Status       string               `json:"status"`
	Guidance     string               `json:"guidance"`
	StartedAt    string               `json:"startedAt"`
	CompletedAt  string               `json:"completedAt,omitempty"`
	Stage        *assessmentStageDTO  `json:"stage,omitempty"`
	Result       *assessmentResultDTO `json:"result,omitempty"`
}

type assessmentSummaryDTO struct {
	AssessmentID   string `json:"assessmentId"`
	Language       string `json:"language"`
	Status         string `json:"status"`
	EstimatedLevel string `json:"estimatedLevel,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Floor          bool   `json:"floor"`
	StartedAt      string `json:"startedAt"`
	CompletedAt    string `json:"completedAt,omitempty"`
}

type profileLevelDTO struct {
	Language     string `json:"language"`
	Level        string `json:"level"`
	AssessmentID string `json:"assessmentId"`
	UpdatedAt    string `json:"updatedAt"`
}

func (h *Handler) handleAssessmentStart(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	body, errResponse := requestBody(req)
	if errResponse != nil {
		return *errResponse
	}
	var document struct {
		Language string `json:"language"`
	}
	if issue := decodeStrict(body, &document, "$"); issue != nil {
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "assessment validation failed", Issues: []issueDTO{*issue}})
	}
	view, err := h.assessments.Start(ctx, inbound.AssessmentStartCommand{Owner: owner, Language: document.Language})
	if err != nil {
		return assessmentError(ctx, err)
	}
	return respondJSON(http.StatusCreated, toAssessmentView(view))
}

func (h *Handler) handleAssessmentAnswer(ctx context.Context, req events.APIGatewayV2HTTPRequest, id string) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	body, errResponse := requestBody(req)
	if errResponse != nil {
		return *errResponse
	}
	var document struct {
		StageIndex *int  `json:"stageIndex"`
		Answers    []int `json:"answers"`
	}
	if issue := decodeStrict(body, &document, "$"); issue != nil {
		return respondJSON(http.StatusBadRequest, validationResponse{Error: "assessment validation failed", Issues: []issueDTO{*issue}})
	}
	if document.StageIndex == nil {
		return errorJSON(http.StatusBadRequest, "stageIndex is required")
	}
	view, err := h.assessments.Answer(ctx, inbound.AssessmentAnswerCommand{
		Owner:        owner,
		AssessmentID: id,
		StageIndex:   *document.StageIndex,
		Answers:      document.Answers,
	})
	if err != nil {
		return assessmentError(ctx, err)
	}
	return respondJSON(http.StatusOK, toAssessmentView(view))
}

func (h *Handler) handleAssessmentGet(ctx context.Context, req events.APIGatewayV2HTTPRequest, id string) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	view, err := h.assessments.Assessment(ctx, owner, id)
	if err != nil {
		return assessmentError(ctx, err)
	}
	return respondJSON(http.StatusOK, toAssessmentView(view))
}

func (h *Handler) handleAssessmentList(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	summaries, err := h.assessments.Assessments(ctx, owner)
	if err != nil {
		return assessmentError(ctx, err)
	}
	items := make([]assessmentSummaryDTO, 0, len(summaries))
	for _, summary := range summaries {
		item := assessmentSummaryDTO{
			AssessmentID:   summary.ID,
			Language:       summary.Language,
			Status:         summary.Status,
			EstimatedLevel: summary.EstimatedLevel,
			Confidence:     summary.Confidence,
			Floor:          summary.Floor,
			StartedAt:      summary.StartedAt.UTC().Format(time.RFC3339Nano),
		}
		if !summary.CompletedAt.IsZero() {
			item.CompletedAt = summary.CompletedAt.UTC().Format(time.RFC3339Nano)
		}
		items = append(items, item)
	}
	return respondJSON(http.StatusOK, map[string]any{"items": items, "guidance": assessmentGuidance})
}

func (h *Handler) handleProfileLevels(ctx context.Context, req events.APIGatewayV2HTTPRequest) events.APIGatewayV2HTTPResponse {
	owner := ownerFrom(req)
	if owner == "" {
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	}
	levels, err := h.assessments.Levels(ctx, owner)
	if err != nil {
		return assessmentError(ctx, err)
	}
	items := make([]profileLevelDTO, 0, len(levels))
	for _, level := range levels {
		items = append(items, profileLevelDTO{
			Language:     level.Language,
			Level:        level.Level,
			AssessmentID: level.AssessmentID,
			UpdatedAt:    level.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return respondJSON(http.StatusOK, map[string]any{"levels": items})
}

func toAssessmentView(view inbound.AssessmentView) assessmentViewDTO {
	dto := assessmentViewDTO{
		AssessmentID: view.ID,
		Language:     view.Language,
		Status:       view.Status,
		Guidance:     assessmentGuidance,
		StartedAt:    view.StartedAt.UTC().Format(time.RFC3339Nano),
	}
	if !view.CompletedAt.IsZero() {
		dto.CompletedAt = view.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	if view.Stage != nil {
		items := make([]assessmentItemDTO, 0, len(view.Stage.Items))
		for _, item := range view.Stage.Items {
			items = append(items, assessmentItemDTO{Kind: item.Kind, Prompt: item.Prompt, Options: item.Options})
		}
		dto.Stage = &assessmentStageDTO{
			Index:     view.Stage.Index,
			Band:      view.Stage.Band,
			BandCount: view.Stage.BandCount,
			Items:     items,
		}
	}
	if view.Result != nil {
		bands := make([]assessmentBandDTO, 0, len(view.Result.Bands))
		for _, band := range view.Result.Bands {
			bands = append(bands, assessmentBandDTO{Band: band.Band, Correct: band.Correct, Total: band.Total, Passed: band.Passed})
		}
		dto.Result = &assessmentResultDTO{
			EstimatedLevel: view.Result.EstimatedLevel,
			Confidence:     view.Result.Confidence,
			Floor:          view.Result.Floor,
			Bands:          bands,
		}
	}
	return dto
}

func assessmentError(ctx context.Context, err error) events.APIGatewayV2HTTPResponse {
	switch {
	case errors.Is(err, lesson.ErrInvalidOwner):
		return errorJSON(http.StatusUnauthorized, "missing authenticated user")
	case errors.Is(err, assessment.ErrInsufficientReference):
		return errorJSON(http.StatusBadRequest, "placement tests are not available for this language yet")
	case errors.Is(err, assessment.ErrInvalidAssessment), errors.Is(err, assessment.ErrInvalidAnswer),
		errors.Is(err, reference.ErrInvalidLanguage), errors.Is(err, reference.ErrInvalidLevel):
		return errorJSON(http.StatusBadRequest, err.Error())
	case errors.Is(err, assessment.ErrAlreadyCompleted):
		return errorJSON(http.StatusConflict, err.Error())
	case errors.Is(err, assessment.ErrConflict):
		return errorJSON(http.StatusConflict, err.Error())
	case errors.Is(err, assessment.ErrNotFound):
		return errorJSON(http.StatusNotFound, err.Error())
	}
	slog.ErrorContext(ctx, "assessment request failed", "error", err)
	return errorJSON(http.StatusInternalServerError, "internal error")
}
