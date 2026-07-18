package lesson

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

type Issue struct {
	Path    string
	Message string
}

type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 1 {
		return fmt.Sprintf("lesson validation failed: %s: %s", e.Issues[0].Path, e.Issues[0].Message)
	}
	return fmt.Sprintf("lesson validation failed: %d issues", len(e.Issues))
}

var (
	uuidPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	controlCharPattern = regexp.MustCompile("[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")
	markupPattern      = regexp.MustCompile(`<[ \t]*[a-zA-Z!/]`)
)

func ValidID(id string) bool {
	return uuidPattern.MatchString(id)
}

type collector struct {
	issues []Issue
}

func (c *collector) add(path, message string, args ...any) {
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	c.issues = append(c.issues, Issue{Path: path, Message: message})
}

func (c *collector) text(path, value string, maxRunes int, required bool) {
	if value == "" {
		if required {
			c.add(path, "must not be empty")
		}
		return
	}
	if utf8.RuneCountInString(value) > maxRunes {
		c.add(path, "must be at most %d characters", maxRunes)
	}
	if controlCharPattern.MatchString(value) {
		c.add(path, "must not contain control characters")
	}
	if markupPattern.MatchString(value) {
		c.add(path, "must not contain HTML tags or markup")
	}
}

func (c *collector) err() error {
	if len(c.issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: c.issues}
}
