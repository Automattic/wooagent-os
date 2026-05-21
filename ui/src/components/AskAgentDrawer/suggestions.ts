// Per-agent + per-page suggestion seeds for the Ask Agent drawer.
//
// Critical rule: every suggestion here is a query the corresponding
// agent prompt's hand-trace eval passes. If a suggestion makes a promise
// the LLM can't deliver on, the operator stops trusting the affordance
// — pull the suggestion before adding it.
//
// Dynamic page-aware generation is a follow-up (DSGWOO-1357 B5); this
// file is the static map that ships in v1.

import type { AskAgent } from '../../api/client';

/** CoS suggestions vary by page since CoS reads across the whole
 *  surface (queue, board, runs, agents). */
const CHIEF_OF_STAFF_BY_PAGE: Record<string, string[]> = {
  'needs-review': [
    'What needs my attention first?',
    'What’s stuck?',
    'Summarize the queue in one sentence.',
  ],
  'board': [
    'What did the agents do overnight?',
    'What’s pending right now?',
  ],
  'done': [
    'What did I approve yesterday?',
    'Anything rejected this week?',
  ],
  'runs': [
    'Show me the most recent runs across all personas.',
    'Did anything fail recently?',
  ],
  'agents': [
    'What is each agent for?',
    'Which specialists are available?',
  ],
};

const CHIEF_OF_STAFF_DEFAULT = [
  'What needs my attention?',
  'What did the agents do today?',
];

/** Specialist suggestions don't vary by page in v1 — the work the
 *  operator can ask each specialist for is the same wherever they
 *  opened the drawer. Page-aware dynamic suggestions land in 1357. */
const SPECIALIST_SUGGESTIONS: Record<Exclude<AskAgent, 'chief_of_staff'>, string[]> = {
  marketing: [
    'What did you draft this week?',
    'What’s our voice for towels?',
  ],
  pricing: [
    'What price changes have you recommended this month?',
    'Look at the Linen Napkin pricing.',
  ],
  'sales-support': [
    'What customer notes did you draft last week?',
    'What’s our usual response to shipping delays?',
  ],
};

/** Returns the suggestion list to show when the active agent's thread
 *  is empty. CoS picks per-page; specialists ignore page and return a
 *  short agent-specific set. */
export function suggestionsForAgentAndPage(agent: AskAgent, page: string): string[] {
  if (agent === 'chief_of_staff') {
    return CHIEF_OF_STAFF_BY_PAGE[page] ?? CHIEF_OF_STAFF_DEFAULT;
  }
  return SPECIALIST_SUGGESTIONS[agent] ?? [];
}

/** @deprecated Use suggestionsForAgentAndPage. Kept as a thin shim
 *  during the transition; will be removed once no callers reference it. */
export function suggestionsForPage(page: string): string[] {
  return suggestionsForAgentAndPage('chief_of_staff', page);
}
