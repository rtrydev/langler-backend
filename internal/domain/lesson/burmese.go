package lesson

import (
	"fmt"
	"strings"

	"github.com/rtrydev/langler-backend/internal/domain/burmese"
)

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
