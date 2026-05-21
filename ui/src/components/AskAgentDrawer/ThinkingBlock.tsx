import { Spinner } from '@wordpress/components';
import type { ThinkingEvent } from '../../lib/useAskAgentThinking';

interface Props {
  events: ThinkingEvent[];
}

// ThinkingBlock renders the mid-flight progress affordance for slow
// tools (DSGWOO-1356). Sits inline in the chat between the operator's
// most recent user turn and the assistant's eventual reply, replacing
// the plain "Thinking…" indicator once the daemon emits at least one
// SSE event. Each event becomes one stacked row inside the block —
// keeps a 4-search Pricing benchmark legible.
//
// The component is purely presentational: parent decides when to mount
// it (typically: `isLoading && events.length > 0`). When events is
// empty but a request is in flight, the parent renders the fallback
// generic spinner from ChatThread instead.

export default function ThinkingBlock({ events }: Props) {
  if (events.length === 0) return null;
  return (
    <div className="wa-chat-thinking-block" aria-live="polite">
      <span className="wa-chat-thinking-block__spinner" aria-hidden="true">
        <Spinner />
      </span>
      <div className="wa-chat-thinking-block__rows">
        {events.map((e, i) => (
          <span key={i} className="wa-chat-thinking-block__row">
            {messageFor(e)}
          </span>
        ))}
      </div>
    </div>
  );
}

/** UI-side fallback prose generator. Prefers daemon-supplied `message`
 *  when non-empty; otherwise builds a default from (agent, tool). Lets
 *  the UI evolve copy without a daemon redeploy. */
function messageFor(e: ThinkingEvent): string {
  if (e.message) return e.message;
  if (e.tool === 'web_search') {
    if (e.agent === 'pricing') return 'Pricing is researching — about a minute.';
    return 'Searching the web — this can take up to a minute.';
  }
  return `Calling ${e.tool}…`;
}
