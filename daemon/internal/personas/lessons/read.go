package lessons

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

const killSwitchEnv = "WOOAGENT_PERSONA_LESSONS_DISABLED"

// DisabledFor reports whether the kill switch lists persona (comma-separated,
// whitespace-tolerant).
func DisabledFor(persona string) bool {
	for _, p := range strings.Split(os.Getenv(killSwitchEnv), ",") {
		if strings.TrimSpace(p) == persona {
			return true
		}
	}
	return false
}

// LoadFor returns the formatted lessons block for persona, ready to prepend to
// a draft prompt — or "" when there is no row, the text is empty, or the kill
// switch disables this persona. Never errors on the disabled/empty paths.
func LoadFor(ctx context.Context, db *sql.DB, persona string) (string, error) {
	if DisabledFor(persona) {
		return "", nil
	}
	row, err := loadRow(ctx, db, persona)
	if err != nil {
		return "", err
	}
	if row == nil || strings.TrimSpace(row.LessonsText) == "" {
		return "", nil
	}
	return formatBlock(row), nil
}

func formatBlock(r *Row) string {
	return fmt.Sprintf("Lessons from recent operator dismissals (%d dismissals · last %d days):\n%s",
		r.SourceCount, spanDays(r.SourceOldest, r.SourceNewest), strings.TrimSpace(r.LessonsText))
}

// spanDays returns the whole-day span between two RFC3339 timestamps, or 0 on
// parse failure (the header still renders).
func spanDays(oldest, newest string) int {
	o, err1 := time.Parse(time.RFC3339, oldest)
	n, err2 := time.Parse(time.RFC3339, newest)
	if err1 != nil || err2 != nil || n.Before(o) {
		return 0
	}
	return int(n.Sub(o).Hours() / 24)
}
