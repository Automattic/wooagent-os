import { useEffect, useMemo, useRef, useState } from 'react';
import { Icon, close } from '@wordpress/icons';
import { Notice } from '@wordpress/components';
import { Text } from '@wordpress/ui';
import ChatThread from './ChatThread';
import MessageInput from './MessageInput';
import Picker from './Picker';
import { suggestionsForPage } from './suggestions';
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

// AskAgentDrawer is the multi-turn chat surface (DSGWOO-1348 B1). The
// operator opens it with ⌘K, types a question or work request, and
// the daemon's CoS agent answers — read questions inline, dispatch
// requests as receipts with a "Working" chip that links to the run.
//
// State here is in-memory per session: the messages array, the active
// agent (CoS-only in B1; B2 unlocks the picker), and a stable
// thread_id minted on mount. The bearer connection is threaded
// through props because the drawer needs it for the api.ask call —
// the App owns the durable Connection.
export default function AskAgentDrawer({ isOpen, onClose, connection }: Props) {
  const [activeAgent /* setActiveAgent — B2 */] = useState<AskAgent>('chief_of_staff');
  const [messages, setMessages] = useState<AskMessage[]>([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const threadId = useMemo(() => crypto.randomUUID(), []);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const getPageContext = useAskAgentContextGetter();

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

  const submit = async (textOverride?: string) => {
    const text = (textOverride ?? input).trim();
    if (!text || isLoading) return;

    const ctx = getPageContext();
    const userMsg: AskMessage = {
      role: 'user',
      content: text,
      page_context: ctx,
    };
    const next = [...messages, userMsg];
    setMessages(next);
    setInput('');
    setIsLoading(true);
    setError(null);

    try {
      const resp = await api.ask(connection, {
        agent: activeAgent,
        thread_id: threadId,
        messages: next,
      });
      setMessages([...next, resp.message]);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsLoading(false);
      // Refocus the input so the next turn is one keystroke away.
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  };

  // Page label drives which suggestion set we surface on an empty
  // thread. Reads the getter so the suggestions reflect the page
  // the operator is currently looking at (not whatever page was
  // active when the drawer last rendered).
  const pageLabel = isOpen ? getPageContext().page : '';
  const suggestions = suggestionsForPage(pageLabel);

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

        <Picker active={activeAgent} />

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
