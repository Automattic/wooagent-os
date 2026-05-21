import { useEffect, useRef } from 'react';
import { Stack } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import Message from './Message';
import ThinkingBlock from './ThinkingBlock';
import type { AskMessage, Connection } from '../../api/client';
import type { ThinkingEvent } from '../../lib/useAskAgentThinking';

interface Props {
  messages: AskMessage[];
  isLoading: boolean;
  /** Mid-flight thinking events from the daemon's SSE stream
   *  (DSGWOO-1356). When non-empty + isLoading, the chat renders a
   *  ThinkingBlock between the last user turn and the eventual
   *  assistant reply — replaces the generic "Thinking…" fallback. */
  thinking?: ThinkingEvent[];
  /** Bearer connection threaded into Message → DispatchChip so the
   *  chip can poll /v1/runs/{id} for terminal state updates. */
  connection: Connection;
  onChipNavigated: () => void;
}

// ChatThread renders the scrolling message column inside the drawer.
// Auto-scrolls to the bottom on new turns (typical chat UX); doesn't
// rebind the scroll position when the operator deliberately scrolls
// up to review earlier turns (would be a regression but isn't blocked
// here — the simple bottom-pin works for v1).
export default function ChatThread({
  messages,
  isLoading,
  thinking = [],
  connection,
  onChipNavigated,
}: Props) {
  const endRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [messages.length, isLoading, thinking.length]);

  // SSE-driven block replaces the generic spinner once at least one
  // thinking event arrives. Fast queries (CoS reads, list_proposals)
  // never trigger a slow-tool event, so the fallback spinner keeps the
  // pre-DSGWOO-1356 chat feel.
  const showThinkingBlock = isLoading && thinking.length > 0;
  const showFallbackSpinner = isLoading && thinking.length === 0;

  return (
    <div className="wa-chat-thread">
      <Stack direction="column" gap="md">
        {messages.map((msg, i) => (
          <Message
            key={i}
            message={msg}
            connection={connection}
            onChipNavigated={onChipNavigated}
          />
        ))}
        {showThinkingBlock && <ThinkingBlock events={thinking} />}
        {showFallbackSpinner && (
          // CUSTOM: inline fast-path thinking indicator. (a) Spinner +
          // label fit the chat-bubble visual language; a stand-alone
          // WPDS <Spinner> floating in the column reads as broken.
          // (b) Plain div + WPDS Spinner. (c) Will be retired if/when
          // every supported tool emits a thinking event.
          <div className="wa-chat-thinking" aria-live="polite">
            <span className="wa-chat-thinking__spinner" aria-hidden="true">
              <Spinner />
            </span>
            <span>Thinking…</span>
          </div>
        )}
        <div ref={endRef} />
      </Stack>
    </div>
  );
}
