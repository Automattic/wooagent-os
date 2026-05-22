import { useEffect, useMemo, useRef, useState } from 'react';
import { Icon, close } from '@wordpress/icons';
import { Notice } from '@wordpress/components';
import { Text } from '@wordpress/ui';
import {
  AgentUIContainer,
  AgentUIMessages,
  AgentUIInput,
} from '@automattic/agenttic-ui';
import Picker from './Picker';
import ThinkingBlock from './ThinkingBlock';
import { adaptMessages } from './adaptMessages';
import {
  api,
  type AskAgent,
  type AskMessage,
  type Connection,
} from '../../api/client';
import { useAskAgentContextGetter } from '../../lib/askAgent';
import { useAskAgentThinking } from '../../lib/useAskAgentThinking';
import { useAskAgentSuggestions } from '../../lib/useAskAgentSuggestions';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  connection: Connection;
}

// AskAgentDrawer is the multi-turn chat surface (DSGWOO-1347). The
// operator opens it with ⌘K, types a question or work request, and the
// picked agent answers — read questions inline, dispatch requests as
// receipts with a "Working" chip that links to the run.
//
// State here is in-memory per session: messages keyed per agent so each
// thread persists when the picker switches, a stable thread_id minted
// on mount, and an `unread` flag set when a non-active agent's thread
// gets a new message. Threads clear on reload — persistence is a
// follow-up.
//
// The inner chat (messages list + input) is composed from
// @automattic/agenttic-ui — see DESIGN.md for the rationale. The drawer
// shell, persona Picker, suggestions row, ThinkingBlock and chip pills
// stay WooAgent-specific.

/** Empty per-agent thread map. One key per live agent; new agents added
 *  here must also be exported from the daemon's AgentSlug enum so the
 *  POST /v1/ask handler routes them. */
const EMPTY_THREADS: Record<AskAgent, AskMessage[]> = {
  chief_of_staff: [],
  marketing: [],
  pricing: [],
  'sales-support': [],
};

const AGENT_LABELS: Record<AskAgent, string> = {
  chief_of_staff: 'Chief of Staff',
  marketing: 'Marketing',
  pricing: 'Pricing',
  'sales-support': 'Sales Support',
};

export default function AskAgentDrawer({ isOpen, onClose, connection }: Props) {
  const [activeAgent, setActiveAgent] = useState<AskAgent>('chief_of_staff');
  // Per-agent message threads. Switching the picker swaps which thread
  // Agenttic UI renders without losing the others — within one session,
  // a half-finished Pricing chat survives a quick detour to CoS.
  const [threads, setThreads] = useState<Record<AskAgent, AskMessage[]>>(EMPTY_THREADS);
  const [unread, setUnread] = useState<Partial<Record<AskAgent, boolean>>>({});
  const [input, setInput] = useState('');
  // Per-agent loading flag so two threads aren't gated by each other —
  // currently the drawer only allows one in-flight request at a time
  // since the input belongs to the active thread, but tracking
  // per-agent keeps the surface honest if we ever lift that constraint.
  const [loadingAgent, setLoadingAgent] = useState<AskAgent | null>(null);
  const [error, setError] = useState<string | null>(null);

  const threadId = useMemo(() => crypto.randomUUID(), []);
  const drawerRef = useRef<HTMLElement | null>(null);
  const getPageContext = useAskAgentContextGetter();

  const messages = threads[activeAgent];
  const isLoading = loadingAgent === activeAgent;

  // SSE-driven mid-flight progress for slow tools (Pricing's
  // web_search benchmark in particular). The hook opens an EventSource
  // when isLoading transitions true and closes when it goes false.
  // Events render as a transient ThinkingBlock between Messages and the
  // input; the bare Agenttic "Thinking…" indicator covers the no-event
  // fast-path.
  const thinking = useAskAgentThinking(connection, threadId, isLoading);

  // Focus the textarea when the drawer opens. AgentUIContainer owns the
  // textarea ref internally — we query for it via the `chat-input` slot.
  useEffect(() => {
    if (!isOpen) return;
    const id = setTimeout(() => {
      drawerRef.current
        ?.querySelector<HTMLTextAreaElement>('[data-slot="chat-input"] textarea')
        ?.focus();
    }, 0);
    return () => clearTimeout(id);
  }, [isOpen, activeAgent]);

  // Esc closes the drawer.
  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isOpen, onClose]);

  // Clear the unread dot for the agent the operator just switched to.
  useEffect(() => {
    setUnread((prev) => {
      if (!prev[activeAgent]) return prev;
      const next = { ...prev };
      delete next[activeAgent];
      return next;
    });
  }, [activeAgent]);

  const submit = async (text: string) => {
    const trimmed = text.trim();
    if (!trimmed || isLoading) return;

    const agent = activeAgent;
    const ctx = getPageContext();
    const userMsg: AskMessage = {
      role: 'user',
      content: trimmed,
      page_context: ctx,
    };
    const nextThread = [...threads[agent], userMsg];
    setThreads((prev) => ({ ...prev, [agent]: nextThread }));
    setInput('');
    setLoadingAgent(agent);
    setError(null);

    try {
      const resp = await api.ask(connection, {
        agent,
        thread_id: threadId,
        messages: nextThread,
      });
      setThreads((prev) => ({
        ...prev,
        [agent]: [...prev[agent], resp.message],
      }));
      // If the operator switched away before the reply came back, flag
      // the originating agent's thread as unread so the picker dot
      // surfaces the new message.
      if (agent !== activeAgent) {
        setUnread((prev) => ({ ...prev, [agent]: true }));
      }
    } catch (err) {
      // Roll back the user message — keeping it without a reply leaves
      // the thread confusingly mid-air. Restore the text in the input so
      // the operator can edit and re-submit.
      setThreads((prev) => ({ ...prev, [agent]: prev[agent].slice(0, -1) }));
      setInput(trimmed);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoadingAgent((prev) => (prev === agent ? null : prev));
    }
  };

  // Page label + active agent drive which suggestion set we surface on
  // an empty thread. The hook fetches from the daemon's queue-aware
  // endpoint (60s TTL) and falls through to the static UI map on
  // failure or for specialists.
  const pageLabel = isOpen ? getPageContext().page : '';
  const suggestions = useAskAgentSuggestions(connection, activeAgent, pageLabel);

  const adapted = useMemo(
    () => adaptMessages(messages, connection, onClose),
    [messages, connection, onClose],
  );

  // SSE-driven ThinkingBlock replaces Agenttic's bare "Thinking…" once
  // at least one thinking event arrives. When events are empty but
  // isLoading is true, we let AgentUIContainer render its own indicator.
  const showThinkingBlock = isLoading && thinking.length > 0;
  const isProcessingForAgenttic = isLoading && thinking.length === 0;

  return (
    <>
      <div
        className={`wa-drawer-scrim${isOpen ? ' is-open' : ''}`}
        onClick={onClose}
        aria-hidden="true"
      />
      <aside
        ref={drawerRef}
        className={`wa-drawer${isOpen ? ' is-open' : ''}`}
        role="dialog"
        aria-label="Ask agent"
        aria-hidden={!isOpen}
      >
        <div className="wa-drawer__header">
          <Text variant="heading-sm">Ask agent</Text>
          {/* CUSTOM: drawer-chrome close button using shared .wa-icon-btn class. (a) WPDS <Button icon={close} variant="tertiary"> doesn't match drawer-header size/padding. (b) Icon-only close with .wa-icon-btn shared chrome. (c) Follow-up: migrate when .wa-icon-btn retires. */}
          <button
            type="button"
            className="wa-icon-btn"
            onClick={onClose}
            aria-label="Close"
          >
            <Icon icon={close} size={18} />
          </button>
        </div>

        <Picker active={activeAgent} onSelect={setActiveAgent} unread={unread} />

        <AgentUIContainer
          variant="embedded"
          className="agenttic wa-chat-stack"
          messages={adapted}
          isProcessing={isProcessingForAgenttic}
          onSubmit={submit}
          inputValue={input}
          onInputChange={setInput}
          placeholder={`Ask ${AGENT_LABELS[activeAgent]}`}
          maxInputLength={2000}
        >
          <AgentUIMessages />

          {showThinkingBlock && (
            <div className="wa-chat-thinking-wrap">
              <ThinkingBlock events={thinking} />
            </div>
          )}

          {error && (
            <div className="wa-chat-error">
              <Notice
                status="error"
                isDismissible
                onRemove={() => setError(null)}
              >
                {error}
              </Notice>
            </div>
          )}

          {messages.length === 0 && suggestions.length > 0 && (
            <div className="wa-chat-suggestions">
              <span className="wa-eyebrow">Suggested</span>
              {suggestions.map((s) => (
                <button
                  key={s}
                  type="button"
                  className="wa-suggestion-row"
                  onClick={() => submit(s)}
                >
                  <span className="wa-suggestion-row__label">{s}</span>
                </button>
              ))}
            </div>
          )}

          <AgentUIInput />
        </AgentUIContainer>
      </aside>
    </>
  );
}
