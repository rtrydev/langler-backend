package lesson

import (
	"fmt"
	"unicode"
)

func japaneseIssues(c *collector, l Lesson) {
	for i, e := range l.Exercises {
		path := fmt.Sprintf("exercises[%d].payload", i)
		switch {
		case e.Cloze != nil:
			requireJapanese(c, path+".text", e.Cloze.Text)
		case e.Reading != nil:
			requireJapanese(c, path+".passage", e.Reading.Passage)
		case e.Translation != nil:
			requireJapanese(c, path+".source", e.Translation.Source)
		case e.Ordering != nil:
			for j, item := range e.Ordering.Items {
				requireJapanese(c, fmt.Sprintf("%s.items[%d]", path, j), item)
			}
		case e.ScriptPractice != nil:
			for j, item := range e.ScriptPractice.Items {
				requireJapanese(c, fmt.Sprintf("%s.items[%d].glyph", path, j), item.Glyph)
			}
		}
	}
}

func requireJapanese(c *collector, path, value string) {
	if value == "" || containsJapanese(value) {
		return
	}
	c.add(path, "must contain Japanese script in a Japanese lesson")
}

func containsJapanese(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}
