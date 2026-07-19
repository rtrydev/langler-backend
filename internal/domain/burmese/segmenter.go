package burmese

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

type NgramModel struct {
	Unigram map[string]int
	Bigram  map[string]map[string]int
	Total   float64
}

type Segmenter struct {
	model NgramModel
}

type candidate struct {
	score  float64
	tokens []string
}

func NewSegmenter(model NgramModel) (*Segmenter, error) {
	if model.Total <= 0 {
		return nil, errors.New("ngram total must be positive")
	}
	if len(model.Unigram) == 0 {
		return nil, errors.New("unigram model must not be empty")
	}
	if model.Bigram == nil {
		model.Bigram = map[string]map[string]int{}
	}
	return &Segmenter{model: model}, nil
}

func (s *Segmenter) Segment(text string) []string {
	prepared := strings.TrimSpace(strings.ReplaceAll(text, " ", ""))
	if prepared == "" {
		return nil
	}
	runes := []rune(prepared)
	cache := make(map[string]candidate)
	return s.viterbi(runes, 0, "<S>", cache).tokens
}

func (s *Segmenter) viterbi(text []rune, start int, previous string, cache map[string]candidate) candidate {
	if start == len(text) {
		return candidate{}
	}
	key := previous + "\x00" + strconv.Itoa(start)
	if cached, ok := cache[key]; ok {
		return cached
	}
	limit := min(len(text)-start, 20)
	var best candidate
	hasBest := false
	for length := 1; length <= limit; length++ {
		word := string(text[start : start+length])
		remainder := s.viterbi(text, start+length, word, cache)
		current := candidate{score: math.Log10(s.probability(previous, word)) + remainder.score, tokens: append([]string{word}, remainder.tokens...)}
		if !hasBest || compare(current, best) > 0 {
			best = current
			hasBest = true
		}
	}
	cache[key] = best
	return best
}

func (s *Segmenter) probability(previous, current string) float64 {
	if following, ok := s.model.Bigram[previous]; ok {
		if pair, ok := following[current]; ok {
			if count, ok := s.model.Unigram[previous]; ok && count > 0 {
				return float64(pair) / float64(count)
			}
		}
	}
	if count, ok := s.model.Unigram[current]; ok {
		return float64(count) / s.model.Total
	}
	return 10 / (s.model.Total * math.Pow10(len([]rune(current))))
}

func compare(left, right candidate) int {
	if left.score > right.score {
		return 1
	}
	if left.score < right.score {
		return -1
	}
	for index := range min(len(left.tokens), len(right.tokens)) {
		if left.tokens[index] > right.tokens[index] {
			return 1
		}
		if left.tokens[index] < right.tokens[index] {
			return -1
		}
	}
	return len(left.tokens) - len(right.tokens)
}
