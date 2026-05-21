import { useEffect, useRef } from 'react';
import { Stack } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import Message from './Message';
import type { AskMessage } from '../../api/client';

interface Props {
  messages: AskMessage[];
  isLoading: boolean;
  onChipNavigated: () => void;
}

// ChatThread renders the scrolling message column inside the drawer.
// Auto-scrolls to the bottom on new turns (typical chat UX); doesn't
// rebind the scroll position when the operator deliberately scrolls
// up to review earlier turns (would be a regression but isn't blocked
// here — the simple bottom-pin works for v1).
export default function ChatThread({ messages, isLoading, onChipNavigated }: Props) {
  const endRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [messages.length, isLoading]);

  return (
    <div className="wa-chat-thread">
      <Stack direction="column" gap="md">
        {messages.map((msg, i) => (
          <Message key={i} message={msg} onChipNavigated={onChipNavigated} />
        ))}
        {isLoading && (
          // CUSTOM: inline thinking indicator while the LLM call is
          // in flight. (a) Spinner + label fit the chat-bubble visual
          // language; a stand-alone <Spinner> floating in the column
          // reads as broken. (b) Plain div + WPDS Spinner. (c) Will
          // be replaced by the SSE-driven ThinkingBlock once
          // DSGWOO-1348 A4 + B3 land (mid-flight per-tool progress).
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
