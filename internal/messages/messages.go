package messages

import (
	"fmt"
	"strings"
)

type StackEntry struct {
	FileName string `json:"file_name"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Name     string `json:"name"`
}

func FormatUserMessage(message string, frame string, stackTrace []StackEntry, advice string) string {
	var builder strings.Builder

	if message != "" {
		builder.WriteString(message)
	}

	if frame != "" {
		builder.WriteString("\n")
		builder.WriteString(frame)
	}

	if len(stackTrace) > 0 {
		for i := len(stackTrace) - 1; i >= 0; i-- {
			stackEntry := stackTrace[i]
			builder.WriteString("\n")
			if stackEntry.Name != "" {
				builder.WriteString(fmt.Sprintf("  at %s (%s:%d:%d)", stackEntry.Name, stackEntry.FileName, stackEntry.Line, stackEntry.Column))
			} else {
				builder.WriteString(fmt.Sprintf("  at %s:%d:%d", stackEntry.FileName, stackEntry.Line, stackEntry.Column))
			}
		}
	}

	if advice != "" {
		builder.WriteString("\n")
		builder.WriteString(advice)
	}

	return builder.String()
}
