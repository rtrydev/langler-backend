package semanticref

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	domain "github.com/rtrydev/langler-backend/internal/domain/reference"
)

const (
	maxIndexBytes   = 64 << 20
	similarityFloor = 0.25
)

var ErrNotConfigured = errors.New("semantic vocab search is not configured")

type Search struct {
	bedrock   *bedrockruntime.Client
	http      *http.Client
	indexURLs map[domain.Language]string
	modelID   string

	mu      sync.Mutex
	indexes map[domain.Language]*vectorIndex
}

type vectorIndex struct {
	dims int
	ids  []string
	vecs []int8
}

func New(bedrock *bedrockruntime.Client, indexURLs map[domain.Language]string, modelID string) (*Search, error) {
	if bedrock == nil {
		return nil, errors.New("bedrock client must not be nil")
	}
	configured := make(map[domain.Language]string, len(indexURLs))
	for language, indexURL := range indexURLs {
		configured[language] = indexURL
	}
	return &Search{
		bedrock:   bedrock,
		http:      &http.Client{Timeout: 10 * time.Second},
		indexURLs: configured,
		modelID:   modelID,
		indexes:   make(map[domain.Language]*vectorIndex),
	}, nil
}

func (s *Search) SimilarVocabIDs(ctx context.Context, language domain.Language, level domain.Level, topic string, limit int) ([]string, error) {
	indexURL := s.indexURLs[language]
	if indexURL == "" || s.modelID == "" {
		return nil, ErrNotConfigured
	}
	index, err := s.loadIndex(ctx, language, indexURL)
	if err != nil {
		return nil, err
	}
	query, err := s.embed(ctx, topic)
	if err != nil {
		return nil, err
	}
	if len(query) != index.dims {
		return nil, fmt.Errorf("query dimensions %d do not match index dimensions %d", len(query), index.dims)
	}
	normalize(query)

	type candidate struct {
		id    string
		score float64
	}
	prefix := string(level) + "#"
	var candidates []candidate
	for i, id := range index.ids {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		vector := index.vecs[i*index.dims : (i+1)*index.dims]
		score := 0.0
		for j, q := range query {
			score += float64(vector[j]) * float64(q)
		}
		score /= 127
		if score >= similarityFloor {
			candidates = append(candidates, candidate{id: id, score: score})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	ids := make([]string, 0, min(limit, len(candidates)))
	for _, entry := range candidates[:min(limit, len(candidates))] {
		ids = append(ids, entry.id)
	}
	return ids, nil
}

func (s *Search) loadIndex(ctx context.Context, language domain.Language, indexURL string) (*vectorIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index := s.indexes[language]; index != nil {
		return index, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build index request: %w", err)
	}
	response, err := s.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch embedding index: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch embedding index: status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxIndexBytes))
	if err != nil {
		return nil, fmt.Errorf("read embedding index: %w", err)
	}

	index, err := parseIndex(raw)
	if err != nil {
		return nil, err
	}
	s.indexes[language] = index
	return index, nil
}

func parseIndex(raw []byte) (*vectorIndex, error) {
	if len(raw) < 4 {
		return nil, errors.New("embedding index is truncated")
	}
	headerLen := binary.BigEndian.Uint32(raw[:4])
	if uint64(len(raw)) < 4+uint64(headerLen) {
		return nil, errors.New("embedding index header is truncated")
	}
	var header struct {
		Version int      `json:"version"`
		Dims    int      `json:"dims"`
		Count   int      `json:"count"`
		IDs     []string `json:"ids"`
	}
	if err := json.Unmarshal(raw[4:4+headerLen], &header); err != nil {
		return nil, fmt.Errorf("parse embedding index header: %w", err)
	}
	if header.Version != 1 || header.Dims <= 0 || header.Count != len(header.IDs) {
		return nil, errors.New("embedding index header is invalid")
	}
	blob := raw[4+headerLen:]
	if len(blob) != header.Count*header.Dims {
		return nil, errors.New("embedding index body size mismatch")
	}
	vecs := make([]int8, len(blob))
	for i, b := range blob {
		vecs[i] = int8(b)
	}
	return &vectorIndex{dims: header.Dims, ids: header.IDs, vecs: vecs}, nil
}

func (s *Search) embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"texts":      []string{text},
		"input_type": "search_query",
		"truncate":   "END",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}
	output, err := s.bedrock.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(s.modelID),
		ContentType: aws.String("application/json"),
		Body:        body,
	})
	if err != nil {
		return nil, fmt.Errorf("embed topic: %w", err)
	}
	var parsed struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(output.Body, &parsed); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}
	if len(parsed.Embeddings) != 1 {
		return nil, errors.New("embed response is missing the embedding")
	}
	return parsed.Embeddings[0], nil
}

func normalize(vector []float32) {
	sum := 0.0
	for _, v := range vector {
		sum += float64(v) * float64(v)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return
	}
	for i := range vector {
		vector[i] = float32(float64(vector[i]) / norm)
	}
}
