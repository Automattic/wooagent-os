package lessons

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
)

const (
	maxLessonsChars = 800 // ~200 tokens; digest prompt is told to stay terse
	maxSourceRecs   = 30  // cap dismissals fed into one LLM call
	defaultModel    = "claude-haiku-4-5-20251001"
)

// Digester turns a persona's recent dismissals into a short lessons block.
type Digester interface {
	Digest(ctx context.Context, persona string, recs []SourceRecord) (string, error)
}

// Job is the periodic digest goroutine. One instance, started once.
type Job struct {
	DB        *sql.DB
	Digester  Digester
	Threshold int
	Every     time.Duration
	Personas  []string
	Out       io.Writer
	Now       func() time.Time
}

func (j *Job) now() time.Time {
	if j.Now != nil {
		return j.Now()
	}
	return time.Now()
}

func (j *Job) threshold() int {
	if j.Threshold > 0 {
		return j.Threshold
	}
	return 5
}

func (j *Job) every() time.Duration {
	if j.Every > 0 {
		return j.Every
	}
	return 10 * time.Minute
}

// Start ticks until ctx is cancelled. Fire-and-forget: per-tick errors are
// logged, never returned (mirrors sweeper.Start).
func (j *Job) Start(ctx context.Context) {
	t := time.NewTicker(j.every())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := j.runOnce(ctx); err != nil {
				j.logf("lessons: tick failed: %v", err)
			}
		}
	}
}

// runOnce processes every persona in the allowlist once, serially. Per-persona
// errors are logged and skipped; the tick never aborts mid-list.
func (j *Job) runOnce(ctx context.Context) error {
	for _, persona := range j.Personas {
		if err := j.digestPersona(ctx, persona); err != nil {
			j.logf("lessons: persona=%s digest failed: %v", persona, err)
		}
	}
	return nil
}

func (j *Job) digestPersona(ctx context.Context, persona string) error {
	var watermark string
	if row, err := loadRow(ctx, j.DB, persona); err != nil {
		return err
	} else if row != nil {
		watermark = row.SourceNewest
	}
	newN, err := countNewDismissals(ctx, j.DB, persona, watermark)
	if err != nil {
		return err
	}
	if newN < j.threshold() {
		j.logf("lessons: persona=%s new_dismissals=%d threshold=%d status=below_threshold", persona, newN, j.threshold())
		return nil
	}
	recs, err := fetchRecentDismissals(ctx, j.DB, persona, maxSourceRecs)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		return nil
	}
	text, err := j.Digester.Digest(ctx, persona, recs)
	if err != nil {
		return err // leaves row unchanged; next tick retries
	}
	text = truncateAtWord(strings.TrimSpace(text), maxLessonsChars)
	if text == "" {
		j.logf("lessons: persona=%s status=empty_digest", persona)
		return nil
	}
	row := Row{
		Persona:      persona,
		LessonsText:  text,
		GeneratedAt:  j.now().UTC().Format(time.RFC3339),
		SourceCount:  len(recs),
		SourceOldest: recs[len(recs)-1].DismissedAt, // list is newest-first
		SourceNewest: recs[0].DismissedAt,           // newest == new watermark
	}
	if err := upsertRow(ctx, j.DB, row); err != nil {
		return err
	}
	j.logf("lessons: persona=%s status=digested chars=%d source_count=%d", persona, len(text), len(recs))
	return nil
}

func (j *Job) logf(format string, args ...any) {
	if j.Out != nil {
		fmt.Fprintf(j.Out, "→ "+format+"\n", args...)
	}
}

// truncateAtWord trims s to at most max chars, cutting at the last space so the
// block never ends mid-word.
func truncateAtWord(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut)
}

// ---- production Digester backed by internal/llm/anthropic ----

const marketingDigestPrompt = `You are reviewing why a shop owner rejected AI-drafted product descriptions. ` +
	`From the dismissals below, write 3-5 short bullet points (no more than 800 characters total) capturing concrete, ` +
	`actionable patterns in what they rejected — refer to phrasing, word choice, tone, or angle. ` +
	`Be specific and grounded in the evidence; never invent a rule the operator did not signal. ` +
	`Output only the bullets, no preamble.`

var digestSystemPrompts = map[string]string{
	"marketing": marketingDigestPrompt,
	// pricing / sales_support added in follow-up issues.
}

type anthropicDigester struct {
	client *anthropic.Client
}

// NewAnthropicDigester builds a Digester, or returns nil when apiKey is empty
// (caller then skips wiring the job — no key, no digests).
func NewAnthropicDigester(apiKey, model string) Digester {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	if model == "" {
		model = defaultModel
	}
	return &anthropicDigester{client: anthropic.New(apiKey, model)}
}

func (a *anthropicDigester) Digest(ctx context.Context, persona string, recs []SourceRecord) (string, error) {
	sys, ok := digestSystemPrompts[persona]
	if !ok {
		return "", fmt.Errorf("no digest prompt for persona %q", persona)
	}
	var b strings.Builder
	for i, r := range recs {
		fmt.Fprintf(&b, "[%d] reason=%s\ncomment: %q\nvariant we showed: %q\n\n", i+1, r.Reason, r.Comment, r.Variant)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := a.client.Call(cctx, anthropic.Request{
		MaxTokens: 512,
		System:    sys,
		Messages:  []anthropic.Message{anthropic.UserMessage(b.String())},
	})
	if err != nil {
		return "", fmt.Errorf("digest call: %w", err)
	}
	var out strings.Builder
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			out.WriteString(blk.Text)
		}
	}
	return out.String(), nil
}
