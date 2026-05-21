// Shared types + React context for the Ask Agent drawer (DSGWOO-1348).
//
// Each screen that mounts the AskAgentDrawer publishes its current page
// context — the page identifier and the items visible on screen — via
// useAskAgentContext. The drawer reads the latest snapshot at submit
// time via useAskAgentContextSnapshot and attaches it to the outgoing
// /v1/ask request so referential queries like "this product" or "the
// Linen Napkin one" resolve without an extra tool round-trip.
//
// The store is a ref, not React state: page context is only consumed
// on user-action (submit), never during render, so there's no benefit
// to triggering re-renders when screens push new context. Writers go
// through useEffect so React's render-time-purity rules stay satisfied.

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  type DependencyList,
  type MutableRefObject,
  type PropsWithChildren,
  type ReactElement,
} from 'react';

/** One item visible on the current screen — what the operator could
 *  plausibly mean by "this one" / "the top one" / "the Linen Napkin one".
 *
 *  Kind discriminates so the chat model can pick the right tool when
 *  resolving the reference. State, persona, age are optional surface
 *  data that lets the model answer status questions without making an
 *  extra tool call. */
export interface VisibleItem {
  id: string;
  title: string;
  kind: 'proposal' | 'run' | 'agent' | 'product' | 'order';
  persona?: string;
  state?: string;
  age_seconds?: number;
}

/** Snapshot of the operator's current page. Sent with every /v1/ask
 *  request so the agent has situational awareness. */
export interface PageContext {
  /** Stable page identifier — `'needs-review'`, `'board'`, `'agents'`,
   *  `'proposal-detail'`, `'runs'`, etc. Used by the agent prompt and
   *  by the suggestion list. */
  page: string;
  /** Items currently rendered on screen, with enough metadata to
   *  resolve "this one"–style references without a tool round-trip. */
  visible_items: VisibleItem[];
  /** Optional — the currently-paired store id. Empty when no store. */
  store_id?: string;
}

const EMPTY: PageContext = { page: 'unknown', visible_items: [] };

interface AskAgentContextValue {
  ref: MutableRefObject<PageContext>;
}

const AskAgentContext = createContext<AskAgentContextValue | null>(null);

/** Wraps the App so screens and the drawer share one page-context ref.
 *  Mount once near the top of the tree (above both the routed screens
 *  and AskAgentDrawer). */
export function AskAgentProvider({ children }: PropsWithChildren): ReactElement {
  const ref = useRef<PageContext>(EMPTY);
  return (
    <AskAgentContext.Provider value={{ ref }}>
      {children}
    </AskAgentContext.Provider>
  );
}

/** Publish the current screen's page context. Each screen calls this
 *  with a thunk that builds the PageContext from its data; the hook
 *  re-runs the thunk only when `deps` change (same contract as
 *  useEffect, which is what this is built on).
 *
 *  The latest call wins. When screens swap, the new screen's effect
 *  overwrites the previous screen's context — that's the intended
 *  behavior. */
export function useAskAgentContext(
  fn: () => PageContext,
  deps: DependencyList,
): void {
  const provider = useContext(AskAgentContext);
  if (!provider) {
    throw new Error(
      'useAskAgentContext must be used inside <AskAgentProvider>',
    );
  }
  useEffect(() => {
    provider.ref.current = fn();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}

/** Read the current page context. Called by the Ask Agent drawer at
 *  submit time to attach context to the outgoing /v1/ask request.
 *
 *  This returns the current ref value synchronously and does NOT
 *  subscribe to changes — the drawer doesn't need to re-render when
 *  the page context shifts. If no provider is mounted (shouldn't
 *  happen in practice — App.tsx mounts one at root), returns an empty
 *  context as a safe default. */
export function useAskAgentContextSnapshot(): PageContext {
  const provider = useContext(AskAgentContext);
  if (!provider) return EMPTY;
  return provider.ref.current;
}

/** Return a getter that yields the current page context when invoked.
 *  Use this over useAskAgentContextSnapshot when the value is needed
 *  inside an event handler or async callback that fires after render —
 *  the returned closure reads ref.current at call time, so it always
 *  sees the most recent screen's context, not a stale render-time
 *  snapshot. The function identity is stable across renders. */
export function useAskAgentContextGetter(): () => PageContext {
  const provider = useContext(AskAgentContext);
  if (!provider) return () => EMPTY;
  return () => provider.ref.current;
}
