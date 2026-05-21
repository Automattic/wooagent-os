// Per-page suggestion seeds for the Ask Agent drawer (DSGWOO-1348 B5
// will replace this static map with page-scoped dynamic generation).
//
// Critical rule: every suggestion here is a query the CoS prompt's
// hand-trace eval passes. If a suggestion makes a promise the LLM
// can't deliver on, the operator stops trusting the affordance —
// pull the suggestion before adding it.

const SUGGESTIONS: Record<string, string[]> = {
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

const DEFAULT_SUGGESTIONS = [
  'What needs my attention?',
  'What did the agents do today?',
];

export function suggestionsForPage(page: string): string[] {
  return SUGGESTIONS[page] ?? DEFAULT_SUGGESTIONS;
}
