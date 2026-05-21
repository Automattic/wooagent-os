import { useEffect, useState } from 'react';
import { type Connection } from '../api/client';
import { suggestionsForAgentAndPage } from '../components/AskAgentDrawer/suggestions';
import type { AskAgent } from '../api/client';

/** TTL for the in-memory cache. The endpoint reads queue counts which
 *  shift slowly during a session; one minute is plenty fresh and
 *  cheap enough that the drawer's open-flicker doesn't fire repeat
 *  fetches. */
const CACHE_TTL_MS = 60_000;

interface CacheEntry {
  fetchedAt: number;
  suggestions: string[];
}

/** Cache lives at module scope so multiple drawer mounts during one
 *  session share state. Keyed by `${agent}:${page}`. */
const cache = new Map<string, CacheEntry>();

/** Returns the suggestion list to render on an empty thread, fetched
 *  from the daemon's queue-aware endpoint and cached at 60s. Falls
 *  back to the static UI map if the network call fails (preserving
 *  the v1.0 behavior so a transient 503 doesn't leave the operator
 *  with no affordances).
 *
 *  Hook semantics:
 *  - Fires the request on mount and whenever (agent, page) changes.
 *  - Returns the cached list immediately if fresh.
 *  - Daemon returns an empty list for specialists in v1 — the hook
 *    detects that and falls through to the static UI defaults so
 *    specialists still get their own suggestion set. */
export function useAskAgentSuggestions(
  connection: Connection,
  agent: AskAgent,
  page: string,
): string[] {
  const fallback = suggestionsForAgentAndPage(agent, page);
  const cacheKey = `${agent}:${page}`;
  const cached = cache.get(cacheKey);
  const fresh = cached && Date.now() - cached.fetchedAt < CACHE_TTL_MS ? cached.suggestions : null;

  const [list, setList] = useState<string[]>(fresh ?? fallback);

  useEffect(() => {
    let cancelled = false;
    const existing = cache.get(cacheKey);
    if (existing && Date.now() - existing.fetchedAt < CACHE_TTL_MS) {
      setList(existing.suggestions.length > 0 ? existing.suggestions : fallback);
      return;
    }

    const url = new URL('/v1/ask/suggestions', connection.daemonUrl);
    url.searchParams.set('agent', agent);
    url.searchParams.set('page', page);
    fetch(url.toString(), {
      headers: { Authorization: `Bearer ${connection.token}` },
    })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`status ${r.status}`))))
      .then((body: { suggestions: string[] }) => {
        if (cancelled) return;
        const next = Array.isArray(body.suggestions) ? body.suggestions : [];
        cache.set(cacheKey, { fetchedAt: Date.now(), suggestions: next });
        setList(next.length > 0 ? next : fallback);
      })
      .catch(() => {
        // Network/daemon error → silently fall back to the static
        // map. The drawer is still useful with stale suggestions.
        if (!cancelled) setList(fallback);
      });

    return () => {
      cancelled = true;
    };
    // fallback is recomputed each render but only depends on (agent,
    // page); excluding it from deps keeps the effect from refetching
    // when the parent re-renders for unrelated reasons.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connection.daemonUrl, connection.token, agent, page, cacheKey]);

  return list;
}
