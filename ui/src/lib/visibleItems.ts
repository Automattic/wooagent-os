// Small helpers that turn the data each screen already renders into
// VisibleItem records for the Ask Agent drawer's page_context
// (DSGWOO-1348 B4). The drawer reads these on submit so referential
// queries like "this one" / "the Linen Napkin one" resolve without
// the LLM needing an extra tool round-trip.

import type {
  Ability,
  Batch,
  Issue,
  ModelProvider,
  Persona,
  Run,
  Store,
} from '../api/client';
import type { VisibleItem } from './askAgent';

/** ageSeconds returns the absolute age of an ISO8601 timestamp in
 *  seconds, or 0 if the input is unparseable. Recomputed on every
 *  page-context read so "37 seconds ago" stays current. */
export function ageSeconds(iso: string | null | undefined): number {
  if (!iso) return 0;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return 0;
  return Math.max(0, Math.floor((Date.now() - t) / 1000));
}

export function issueToVisible(i: Issue): VisibleItem {
  return {
    id: i.id,
    title: i.title,
    kind: 'proposal',
    persona: i.persona || undefined,
    state: i.status,
    age_seconds: ageSeconds(i.created_at),
  };
}

export function batchToVisible(b: Batch): VisibleItem {
  return {
    id: b.id,
    // Batches don't have a human title field today — synthesize one
    // that's grounded enough for the model to talk about ("batch
    // of 9 Marketing proposals") without making claims it can't back.
    title: `Batch of ${b.pending + b.approved + b.rejected} ${b.persona} proposals`,
    kind: 'proposal',
    persona: b.persona,
    state: b.pending > 0 ? 'pending' : b.approved > 0 ? 'approved' : 'rejected',
    age_seconds: ageSeconds(b.created_at),
  };
}

export function runToVisible(r: Run): VisibleItem {
  return {
    id: r.id,
    title: `${r.persona} run · ${r.status}`,
    kind: 'run',
    persona: r.persona,
    state: r.status,
    // Runs don't expose created_at on the wire; scheduled_at is the
    // closest approximation (set at enqueue, treated as the run's
    // "born at" timestamp by the runs UI).
    age_seconds: ageSeconds(r.scheduled_at),
  };
}

export function personaToVisible(p: Persona): VisibleItem {
  return {
    id: p.persona,
    title: p.name || p.persona,
    kind: 'agent',
    persona: p.persona,
    state: p.enabled ? 'enabled' : 'disabled',
  };
}

export function storeToVisible(s: Store): VisibleItem {
  return {
    id: s.id,
    title: s.url || s.id,
    kind: 'product', // closest existing kind; settings-page items aren't really proposals
    state: s.status,
  };
}

export function modelProviderToVisible(m: ModelProvider): VisibleItem {
  return {
    id: m.id,
    title: m.name || `${m.kind} · ${m.default_model}`,
    kind: 'product',
    state: m.last_test_status,
  };
}

export function abilityToVisible(a: Ability): VisibleItem {
  return {
    id: a.id,
    title: a.title || a.name || a.id,
    kind: 'product',
    state: a.effective_trust || a.trust_state,
  };
}
