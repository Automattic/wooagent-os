package mcp

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// pressable410 is the real body that prompted DSGWOO-1469, trimmed of a few
// hundred bytes of identical CSS. The shape is what matters: the actionable
// sentence sits inside a <p>, behind a <style> block big enough to swamp any
// naive tag-strip.
const pressable410 = `<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="utf-8">
		<title>410 Gone</title>
		<style>
		body {
			background-color: #f8f8f8;
			font-family: "Helvetica Neue", Helvetica, Arial, sans-serif;
			color: #333;
			padding-top: 20px;
		}
		.error-msg h1 { font-size: 52px; display: block; margin: 0px; }
		.support-msg p { font-size: 14px; color: #888; }
		</style>
	</head>
	<body>
		<div class="container">
			<div class="error-msg">
				<h1>410 Gone</h1>
				<p>This site is disabled.</p>
			</div>
			<div class="support-msg">
				<p>Website owner? If you think you have reached this message in error, please contact support.</p>
			</div>
		</div>
	</body>
</html>`

func TestStatusError_UnwrapsByStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{401, ErrStoreAuth},
		{403, ErrStoreAuth},
		{404, ErrStoreGone},
		{410, ErrStoreGone},
		{429, ErrStoreUnavailable},
		{500, ErrStoreUnavailable},
		{502, ErrStoreUnavailable},
		{400, ErrStoreBadRequest},
		{422, ErrStoreBadRequest},
		{200, nil},
	} {
		err := NewStatusError("https://s.example.com/mcp", tc.status, []byte("x"))
		if tc.want == nil {
			for _, s := range []error{ErrStoreAuth, ErrStoreGone, ErrStoreUnavailable, ErrStoreBadRequest} {
				if errors.Is(err, s) {
					t.Errorf("status %d should match no class, matched %v", tc.status, s)
				}
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: want %v, got %v", tc.status, tc.want, err)
		}
	}
}

func TestStatusError_SurvivesWrapping(t *testing.T) {
	base := NewStatusError("https://s.example.com/mcp", 410, []byte(pressable410))
	wrapped := fmt.Errorf("draft: %w", fmt.Errorf("mcp initialize: %w", base))

	if !errors.Is(wrapped, ErrStoreGone) {
		t.Errorf("wrapped error lost its class: %v", wrapped)
	}
	var target *StatusError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should recover the concrete type")
	}
	if target.Endpoint != "https://s.example.com/mcp" || target.StatusCode != 410 {
		t.Errorf("recovered %+v", target)
	}
	if len(target.Body) != len(pressable410) {
		t.Errorf("Body should stay untruncated: got %d chars, want %d", len(target.Body), len(pressable410))
	}
}

// The heart of DSGWOO-1469: the sentence an operator needs has to survive,
// and the stylesheet must not.
func TestStatusError_ExtractsProseFromHTMLErrorPage(t *testing.T) {
	err := NewStatusError("https://s.example.com/mcp", 410, []byte(pressable410))

	msg := err.Error()
	if !strings.Contains(msg, "This site is disabled") {
		t.Errorf("the actionable sentence was lost: %q", msg)
	}
	for _, css := range []string{"font-family", "background-color", "padding-top", "#f8f8f8", "{"} {
		if strings.Contains(msg, css) {
			t.Errorf("stylesheet leaked into the message (%q): %q", css, msg)
		}
	}
	if strings.Contains(msg, "<") {
		t.Errorf("markup leaked into the message: %q", msg)
	}
	// Bounded — this lands in a run-log table cell.
	if len([]rune(msg)) > bodyExcerptMax+40 {
		t.Errorf("message length %d, want it capped near %d: %q", len([]rune(msg)), bodyExcerptMax, msg)
	}
}

// Detail is what the scheduler folds into failure_reason, so it has to
// honour the caller's budget exactly — that budget is what keeps the
// finished sentence under the UI's show-in-full threshold.
func TestStatusError_DetailRespectsBudget(t *testing.T) {
	err := NewStatusError("https://s.example.com/mcp", 410, []byte(pressable410))

	for _, budget := range []int{20, 60, 90, 400} {
		got := err.Detail(budget)
		// +1 for the ellipsis appended on truncation.
		if len([]rune(got)) > budget+1 {
			t.Errorf("Detail(%d) returned %d runes: %q", budget, len([]rune(got)), got)
		}
	}

	// At a realistic budget the sentence that matters has to survive.
	d := err.Detail(90)
	if !strings.Contains(d, "This site is disabled") {
		t.Errorf("Detail(90) lost the actionable sentence: %q", d)
	}
	if strings.Contains(d, "font-family") {
		t.Errorf("Detail leaked CSS: %q", d)
	}
}

// The <title>/<h1> duplication that error pages produce would otherwise
// spend the excerpt budget restating the status code.
func TestReadableBody_DropsDuplicatedHeading(t *testing.T) {
	got := readableBody("<title>410 Gone</title><h1>410 Gone</h1><p>This site is disabled.</p>", 200)
	if got != "410 Gone This site is disabled." {
		t.Errorf("got %q, want the heading stated once", got)
	}
	// A phrase repeating later in the text is content, not markup noise.
	keep := "Service unavailable. Retry later. Service unavailable."
	if got := readableBody(keep, 200); got != keep {
		t.Errorf("non-leading repetition should be preserved: got %q", got)
	}
}

func TestReadableBody(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"plain text":       {"boom", "boom"},
		"json passthrough": {`{"code":"rest_forbidden"}`, `{"code":"rest_forbidden"}`},
		"collapses whitespace": {
			"line one\n\n\tline two",
			"line one line two",
		},
		"strips tags": {"<p>hello</p>", "hello"},
		"drops script": {
			"<script>var a = 1; if (a < 2) {}</script><p>real message</p>",
			"real message",
		},
		"empty":      {"", ""},
		"whitespace": {"   \n  ", ""},
		"tags only":  {"<div></div>", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := readableBody(tc.in, 200); got != tc.want {
				t.Errorf("readableBody(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReadableBody_CapsWithEllipsis(t *testing.T) {
	got := readableBody(strings.Repeat("ab ", 200), 20)
	if len([]rune(got)) > 21 {
		t.Errorf("length %d, want <= 21: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected an ellipsis marking truncation: %q", got)
	}
}

// A JSON body — the ordinary WP REST shape — must not be mangled by the
// HTML path.
func TestStatusError_JSONBodyUnharmed(t *testing.T) {
	body := `{"code":"rest_forbidden","message":"Sorry, you are not allowed to do that.","data":{"status":401}}`
	err := NewStatusError("https://s.example.com/mcp", 401, []byte(body))
	if !errors.Is(err, ErrStoreAuth) {
		t.Errorf("401 should classify as auth")
	}
	if !strings.Contains(err.Error(), "not allowed to do that") {
		t.Errorf("JSON message should survive: %q", err.Error())
	}
}
