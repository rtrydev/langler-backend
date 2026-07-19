package lesson

import "fmt"

func polishIssues(c *collector, l Lesson) {
	for i, exercise := range l.Exercises {
		if exercise.ScriptPractice == nil {
			continue
		}
		for j, item := range exercise.ScriptPractice.Items {
			if item.Kind == "" {
				c.add(
					fmt.Sprintf("exercises[%d].payload.items[%d].kind", i, j),
					"must be %q or %q for Polish orthography practice",
					ScriptKindChoice,
					ScriptKindDictation,
				)
			}
		}
	}
}
