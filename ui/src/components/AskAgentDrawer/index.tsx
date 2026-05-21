import { useEffect, useMemo, useRef, useState } from 'react';
import { Icon, close } from '@wordpress/icons';
import { Notice } from '@wordpress/components';
import { Text } from '@wordpress/ui';
import ChatThread from './ChatThread';
import MessageInput from './MessageInput';
import Picker from './Picker';
import { suggestionsForAgentAndPage } from './suggestions';
import {
  api,
  type AskAgent,
  type AskMessage,
  type Connection,
} from '../../api/client';
import { useAskAgentContextGetter } from '../../lib/askAgent';

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
// Live agents: Chief of Staff (default), Marketing, Pricing,
// Sales Support. Inventory / Accounting / Reporting appear disabled in
// the picker — their runtimes aren't registered (1355).

/** Empty per-agent thread map. One key per live agent; new agents added
 *  here must also be exported from the daemon's AgentSlug enum so the
 *  POST /v1/ask handler routes them. */
const EMPTY_THREADS: Record<AskAgent, AskMessage[]> = {
  chief_of_staff: [],
  marketing: [],
  pricing: [],
  'sales-support': [],
};

export default function AskAgentDrawer({ isOpen, onClose, connection }: Props) {
  const [activeAgent, setActiveAgent] = useState<AskAgent>('chief_of_staff');
  // Per-agent message threads. Switching the picker swaps which thread
  // ChatThread renders without losing the others — within one session,
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
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const getPageContext = useAskAgentContextGetter();

  const messages = threads[activeAgent];
  const isLoading = loadingAgent === activeAgent;

  // Focus the input when the drawer opens.
  useEffect(() => {
    if (!isOpen) return;
    const id = setTimeout(() => inputRef.current?.focus(), 0);
    return () => clearTimeout(id);
  }, [isOpen]);

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

  const submit = async (textOverride?: string) => {
    const text = (textOverride ?? input).trim();
    if (!text || isLoading) return;

    const agent = activeAgent;
    const ctx = getPageContext();
    const userMsg: AskMessage = {
      role: 'user',
      content: text,
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
      // the thread confusingly mid-air, and the operator can re-submit
      // from the input box (the input itself is empty by now).
      setThreads((prev) => ({ ...prev, [agent]: prev[agent].slice(0, -1) }));
      setInput(text);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoadingAgent((prev) => (prev === agent ? null : prev));
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  };

  // Page label + active agent drive which suggestion set we surface on
  // an empty thread. Reads the getter so suggestions reflect the page
  // the operator is currently looking at (not whatever page was active
  // when the drawer last rendered).
  const pageLabel = isOpen ? getPageContext().page : '';
  const suggestions = suggestionsForAgentAndPage(activeAgent, pageLabel);

  return (
    <>
      <div
        className={`wa-drawer-scrim${isOpen ? ' is-open' : ''}`}
        onClick={onClose}
        aria-hidden="true"
      />
      <aside
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

        <ChatThread
          messages={messages}
          isLoading={isLoading}
          onChipNavigated={onClose}
        />

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

        <MessageInput
          value={input}
          onChange={setInput}
          onSubmit={submit}
          disabled={isLoading}
          textareaRef={inputRef}
        />
      </aside>
    </>
  );
}
