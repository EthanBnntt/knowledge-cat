package knowledge_cat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogEntry represents a single entry in an OKF log.md file.
type LogEntry struct {
	// Date is the date the entry was created.
	Date time.Time
	// Action is a short label describing the type of change.
	// Conventional values: "Update", "Creation", "Deprecation", "Deletion", "Edit".
	Action string
	// Description is the prose description of the change.
	Description string
}

// AppendLog appends an entry to the log.md file at the bundle root.
// If log.md doesn't exist, it creates one with the standard header.
//
// The OKF spec describes log.md format as:
//
//	# Directory Update Log
//
//	## 2026-05-22
//	* **Update**: Description of the change.
func AppendLog(bundlePath string, entry LogEntry) error {
	logPath := filepath.Join(bundlePath, "log.md")

	// Read existing content if file exists.
	var content string
	if data, err := os.ReadFile(logPath); err == nil {
		content = string(data)
	}

	// If no existing content, create header.
	if strings.TrimSpace(content) == "" {
		content = "# Directory Update Log\n\n"
	}

	dateHeading := entry.Date.Format("2006-01-02")
	entryLine := fmt.Sprintf("* **%s**: %s\n", entry.Action, entry.Description)

	// Check if today's date heading already exists.
	dateMarker := "## " + dateHeading + "\n"
	if idx := strings.Index(content, dateMarker); idx >= 0 {
		// Insert the new entry right after the date heading.
		insertAt := idx + len(dateMarker)
		content = content[:insertAt] + entryLine + content[insertAt:]
	} else {
		// Append new date section. Find where to insert — after the header line.
		// Insert after the first blank line (after the # Directory Update Log header).
		headerEnd := strings.Index(content, "\n\n")
		if headerEnd < 0 {
			headerEnd = len(content)
		} else {
			headerEnd += 2 // skip past the blank line
		}

		newSection := fmt.Sprintf("## %s\n%s\n", dateHeading, entryLine)
		content = content[:headerEnd] + newSection + content[headerEnd:]
	}

	return os.WriteFile(logPath, []byte(content), 0o644)
}

// ReadLog reads all entries from the bundle root log.md file.
// Returns nil if log.md doesn't exist or is empty.
func ReadLog(bundlePath string) ([]LogEntry, error) {
	logPath := filepath.Join(bundlePath, "log.md")

	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read log: %w", err)
	}

	return parseLogEntries(string(data))
}

// parseLogEntries parses log.md content into structured entries.
func parseLogEntries(content string) ([]LogEntry, error) {
	var entries []LogEntry
	lines := strings.Split(content, "\n")

	var currentDate time.Time

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match date headings: ## YYYY-MM-DD
		if strings.HasPrefix(line, "## ") && len(line) > 3 {
			dateStr := strings.TrimPrefix(line, "## ")
			dateStr = strings.TrimSpace(dateStr)
			if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
				currentDate = parsed
			}
			continue
		}

		// Match log entries: * **Action**: description
		if strings.HasPrefix(line, "* **") && !currentDate.IsZero() {
			rest := strings.TrimPrefix(line, "* **")
			colonIdx := strings.Index(rest, "**:")
			if colonIdx > 0 {
				action := rest[:colonIdx]
				description := strings.TrimSpace(rest[colonIdx+3:])
				entries = append(entries, LogEntry{
					Date:        currentDate,
					Action:      action,
					Description: description,
				})
			}
		}
	}

	return entries, nil
}
