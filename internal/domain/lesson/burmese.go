package lesson

import (
	"fmt"
	"strings"

	"github.com/rtrydev/langler-backend/internal/domain/burmese"
)

func normalizeBurmese(l *Lesson) {
	n := burmese.Normalize
	l.Title = n(l.Title)
	l.Description = n(l.Description)
	l.Topic = n(l.Topic)
	for i := range l.Exercises {
		e := &l.Exercises[i]
		e.Prompt = n(e.Prompt)
		if e.Cloze != nil {
			e.Cloze.Text = n(e.Cloze.Text)
			for j := range e.Cloze.Blanks {
				b := &e.Cloze.Blanks[j]
				b.Answer = n(b.Answer)
				b.Hint = n(b.Hint)
				normalizeAll(b.Alternates, n)
			}
			normalizeAll(e.Cloze.WordBank, n)
		}
		if e.Translation != nil {
			e.Translation.Source = n(e.Translation.Source)
			e.Translation.Reference = n(e.Translation.Reference)
		}
		if e.Ordering != nil {
			normalizeAll(e.Ordering.Items, n)
			e.Ordering.Translation = n(e.Ordering.Translation)
		}
		if e.Matching != nil {
			for j := range e.Matching.Pairs {
				e.Matching.Pairs[j].Left = n(e.Matching.Pairs[j].Left)
				e.Matching.Pairs[j].Right = n(e.Matching.Pairs[j].Right)
			}
		}
		if e.MultipleChoice != nil {
			for j := range e.MultipleChoice.Questions {
				q := &e.MultipleChoice.Questions[j]
				q.Question = n(q.Question)
				q.Answer = n(q.Answer)
				normalizeAll(q.Options, n)
			}
		}
		if e.Reading != nil {
			e.Reading.Title = n(e.Reading.Title)
			e.Reading.Passage = n(e.Reading.Passage)
			for j := range e.Reading.Annotations {
				a := &e.Reading.Annotations[j]
				a.Surface = n(a.Surface)
				a.Reading = n(a.Reading)
				a.Gloss = n(a.Gloss)
			}
			for j := range e.Reading.Questions {
				q := &e.Reading.Questions[j]
				q.Question = n(q.Question)
				q.Answer = n(q.Answer)
				normalizeAll(q.Options, n)
				normalizeAll(q.Alternates, n)
			}
		}
		if e.WritingPrompt != nil {
			e.WritingPrompt.Guidance = n(e.WritingPrompt.Guidance)
			e.WritingPrompt.ModelAnswer = n(e.WritingPrompt.ModelAnswer)
		}
		if e.ScriptPractice != nil {
			for j := range e.ScriptPractice.Items {
				item := &e.ScriptPractice.Items[j]
				item.Glyph = n(item.Glyph)
				item.Reading = n(item.Reading)
				item.Meaning = n(item.Meaning)
				item.Answer = n(item.Answer)
				normalizeAll(item.Options, n)
			}
		}
	}
}

func normalizeAll(values []string, n func(string) string) {
	for i := range values {
		values[i] = n(values[i])
	}
}

func burmeseIssues(c *collector, l Lesson) {
	for index, exercise := range l.Exercises {
		path := fmt.Sprintf("exercises[%d]", index)
		switch exercise.Type {
		case TypeCloze:
			checkRequiredBurmese(c, path+".payload.text", exercise.Cloze.Text)
			for itemIndex, blank := range exercise.Cloze.Blanks {
				checkBurmese(c, fmt.Sprintf("%s.payload.blanks[%d].answer", path, itemIndex), blank.Answer)
			}
			for itemIndex, word := range exercise.Cloze.WordBank {
				checkBurmese(c, fmt.Sprintf("%s.payload.wordBank[%d]", path, itemIndex), word)
			}
		case TypeReading:
			checkRequiredBurmese(c, path+".payload.title", exercise.Reading.Title)
			checkRequiredBurmese(c, path+".payload.passage", exercise.Reading.Passage)
			for itemIndex, annotation := range exercise.Reading.Annotations {
				checkBurmese(c, fmt.Sprintf("%s.payload.annotations[%d].surface", path, itemIndex), annotation.Surface)
			}
		case TypeTranslation:
			checkRequiredBurmese(c, path+".payload.source", exercise.Translation.Source)
		case TypeOrdering:
			for itemIndex, item := range exercise.Ordering.Items {
				checkRequiredBurmese(c, fmt.Sprintf("%s.payload.items[%d]", path, itemIndex), item)
			}
		case TypeMatching:
			for itemIndex, pair := range exercise.Matching.Pairs {
				checkRequiredBurmese(c, fmt.Sprintf("%s.payload.pairs[%d].left", path, itemIndex), pair.Left)
			}
		case TypeScriptPractice:
			for itemIndex, item := range exercise.ScriptPractice.Items {
				itemPath := fmt.Sprintf("%s.payload.items[%d].glyph", path, itemIndex)
				if !burmese.ContainsMyanmar(item.Glyph) {
					c.add(itemPath, "must contain Burmese script in a Burmese lesson")
				} else if burmese.RuneCount(strings.TrimSpace(item.Glyph)) > 1 {
					checkBurmese(c, itemPath, item.Glyph)
				}
			}
		}
	}
}

func checkRequiredBurmese(c *collector, path, value string) {
	if value != "" && !burmese.ContainsMyanmar(value) {
		c.add(path, "must contain Burmese script in a Burmese lesson")
		return
	}
	checkBurmese(c, path, value)
}

func checkBurmese(c *collector, path, value string) {
	for _, issue := range burmese.ValidateOrthography(value) {
		c.add(path, "contains illegal Burmese orthography: %s", issue.Error())
	}
}
