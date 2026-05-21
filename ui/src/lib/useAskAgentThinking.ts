import { useEffect, useState } from 'react';
import type { Connection } from '../api/client';

/** One thinking event sent by the daemon's SSE stream
 *  (`GET /v1/ask/events`). Mirrors the daemon's `ask.Event` type. */
export interface ThinkingEvent {
  kind: 'thinking';
  agent: string;
  tool: string;
  message: string;
}

/** Opens an EventSource connection to the daemon's thinking-event SSE
 *  stream while `isLoading` is true; closes when it goes false. Returns
 *  the list of events received during the current in-flight request.
 *
 *  Lifecycle:
 *  - When `isLoading` transitions false → true, open the connection and
 *    clear the event list (a new request starts a fresh list).
 *  - While open, events stream in via the message handler.
 *  - When `isLoading` transitions true → false, close the connection.
 *    Events stay rendered until the next submit so the user can still
 *    see context next to the assistant's reply.
 *
 *  Note: EventSource can't set arbitrary headers in browsers; the
 *  daemon's bearer middleware accepts `?token=` as a fallback. The
 *  fallback is documented in `daemon/internal/httpapi/middleware.go`. */
export function useAskAgentThinking(
  connection: Connection,
  threadId: string,
  isLoading: boolean,
): ThinkingEvent[] {
  const [events, setEvents] = useState<ThinkingEvent[]>([]);

  useEffect(() => {
    if (!isLoading) return;
    // Reset on each new in-flight request.
    setEvents([]);

    const url = new URL('/v1/ask/events', connection.daemonUrl);
    url.searchParams.set('thread_id', threadId);
    url.searchParams.set('token', connection.token);

    const src = new EventSource(url.toString());
    src.onmessage = (evt) => {
      try {
        const parsed = JSON.parse(evt.data) as ThinkingEvent;
        if (parsed?.kind === 'thinking') {
          setEvents((prev) => [...prev, parsed]);
        }
      } catch {
        // Malformed frame — ignore. The daemon never sends garbage,
        // but a buggy proxy could insert chrome we can't parse.
      }
    };
    // onerror fires on disconnect too; we don't surface anything to
    // the user (the drawer's loading state is the real signal).
    return () => {
      src.close();
    };
  }, [connection.daemonUrl, connection.token, threadId, isLoading]);

  return events;
}
