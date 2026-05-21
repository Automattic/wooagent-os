package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
)

// fakeEnqueue is the EnqueuerFunc the dispatch tests pass instead of
// a real scheduler. Captures calls + returns canned values.
type fakeEnqueue struct {
	calls   []string
	nextRun scheduler.Run
	nextErr error
}

func (f *fakeEnqueue) call() EnqueuerFunc {
	return func(_ context.Context, persona string) (scheduler.Run, error) {
		f.calls = append(f.calls, persona)
		if f.nextErr != nil {
			return scheduler.Run{}, f.nextErr
		}
		r := f.nextRun
		if r.ID == "" {
			r.ID = "rn_" + persona
		}
		return r, nil
	}
}

func TestDispatchTool_HappyPath(t *testing.T) {
	enq := &fakeEnqueue{nextRun: scheduler.Run{ID: "rn_abc", Persona: "pricing", Trigger: scheduler.TriggerOperatorAsked}}
	d := &DispatchTool{Enqueue: enq.call()}
	raw, err := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"pricing","target":"SKU-1234","brief":"competitor dropped price"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out dispatchOutput
	_ = json.Unmarshal([]byte(raw), &out)
	if !out.OK || out.RunID != "rn_abc" || out.Persona != "pricing" || out.ETASeconds != 60 || out.Target != "SKU-1234" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(enq.calls) != 1 || enq.calls[0] != "pricing" {
		t.Errorf("expected 1 call to pricing, got %v", enq.calls)
	}
}

func TestDispatchTool_ETAPerPersona(t *testing.T) {
	cases := map[string]int{
		"marketing":     10,
		"pricing":       60,
		"sales-support": 15,
	}
	for persona, wantETA := range cases {
		enq := &fakeEnqueue{}
		d := &DispatchTool{Enqueue: enq.call()}
		raw, _ := d.Execute(context.Background(), json.RawMessage(
			`{"persona":"`+persona+`","target":"X","brief":"do thing"}`))
		var out dispatchOutput
		_ = json.Unmarshal([]byte(raw), &out)
		if out.ETASeconds != wantETA {
			t.Errorf("%s: expected eta=%d, got %d", persona, wantETA, out.ETASeconds)
		}
	}
}

func TestDispatchTool_UnknownPersonaIsSoftFail(t *testing.T) {
	enq := &fakeEnqueue{}
	d := &DispatchTool{Enqueue: enq.call()}
	raw, err := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"reporting","target":"Q2","brief":"analyze sales"}`))
	if err != nil {
		t.Fatalf("expected soft fail, got hard error: %v", err)
	}
	var out dispatchOutput
	_ = json.Unmarshal([]byte(raw), &out)
	if out.OK || !strings.Contains(out.Reason, "reporting") || !strings.Contains(out.Reason, "not a dispatchable specialist") {
		t.Errorf("unexpected soft-fail output: %+v", out)
	}
	if len(enq.calls) != 0 {
		t.Errorf("enqueuer should not have been called for unknown persona; got %v", enq.calls)
	}
}

func TestDispatchTool_MissingTargetIsSoftFail(t *testing.T) {
	d := &DispatchTool{Enqueue: (&fakeEnqueue{}).call()}
	raw, _ := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"marketing","brief":"do thing"}`))
	var out dispatchOutput
	_ = json.Unmarshal([]byte(raw), &out)
	if out.OK || !strings.Contains(out.Reason, "target is required") {
		t.Errorf("expected target-required soft fail, got: %+v", out)
	}
}

func TestDispatchTool_MissingBriefIsSoftFail(t *testing.T) {
	d := &DispatchTool{Enqueue: (&fakeEnqueue{}).call()}
	raw, _ := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"marketing","target":"X"}`))
	var out dispatchOutput
	_ = json.Unmarshal([]byte(raw), &out)
	if out.OK || !strings.Contains(out.Reason, "brief is required") {
		t.Errorf("expected brief-required soft fail, got: %+v", out)
	}
}

func TestDispatchTool_SchedulerErrorIsSoftFail(t *testing.T) {
	enq := &fakeEnqueue{nextErr: errors.New("persona disabled")}
	d := &DispatchTool{Enqueue: enq.call()}
	raw, _ := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"marketing","target":"X","brief":"y"}`))
	var out dispatchOutput
	_ = json.Unmarshal([]byte(raw), &out)
	if out.OK || !strings.Contains(out.Reason, "persona disabled") {
		t.Errorf("expected scheduler-error soft fail, got: %+v", out)
	}
}

func TestDispatchTool_NoEnqueueConfiguredHardFails(t *testing.T) {
	d := &DispatchTool{}
	_, err := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"marketing","target":"X","brief":"y"}`))
	if err == nil || !strings.Contains(err.Error(), "enqueuer missing") {
		t.Fatalf("expected hard fail when not configured; got: %v", err)
	}
}

func TestDispatchTool_RateLimitTriggers(t *testing.T) {
	enq := &fakeEnqueue{}
	limiter := NewRateLimiter(60*time.Second, 2)
	d := &DispatchTool{Enqueue: enq.call(), Limiter: limiter}

	for i := 0; i < 2; i++ {
		raw, _ := d.Execute(context.Background(), json.RawMessage(
			`{"persona":"pricing","target":"SKU-1","brief":"check"}`))
		var out dispatchOutput
		_ = json.Unmarshal([]byte(raw), &out)
		if !out.OK {
			t.Fatalf("call %d should have succeeded: %+v", i, out)
		}
	}
	// Third call exceeds the cap.
	raw, _ := d.Execute(context.Background(), json.RawMessage(
		`{"persona":"pricing","target":"SKU-1","brief":"check"}`))
	var out dispatchOutput
	_ = json.Unmarshal([]byte(raw), &out)
	if out.OK || !strings.Contains(out.Reason, "already running") {
		t.Fatalf("expected rate-limit soft fail; got: %+v", out)
	}
	if len(enq.calls) != 2 {
		t.Errorf("expected 2 calls (third rate-limited); got %d", len(enq.calls))
	}
}

func TestRateLimiter_PerKey(t *testing.T) {
	l := NewRateLimiter(60*time.Second, 1)
	now := time.Unix(1_700_000_000, 0)
	if !l.Try("a", now) {
		t.Fatal("first call to a should succeed")
	}
	if l.Try("a", now) {
		t.Fatal("second call to a should be rate-limited")
	}
	if !l.Try("b", now) {
		t.Fatal("first call to b should succeed (separate bucket)")
	}
}

func TestRateLimiter_WindowSliding(t *testing.T) {
	l := NewRateLimiter(60*time.Second, 1)
	t0 := time.Unix(1_700_000_000, 0)
	if !l.Try("a", t0) {
		t.Fatal("first call should succeed")
	}
	if l.Try("a", t0.Add(30*time.Second)) {
		t.Fatal("second within window should be rate-limited")
	}
	if !l.Try("a", t0.Add(61*time.Second)) {
		t.Fatal("call past window should succeed")
	}
}
