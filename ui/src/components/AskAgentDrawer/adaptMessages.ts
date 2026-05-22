import type { ComponentType } from 'react';
import type { AskMessage, Connection } from '../../api/client';
import AssistantChips from './AssistantChips';

// Structural mirror of @automattic/agenttic-ui's Message type.
// Imported as a local type because the package re-exports `Message`
// as both a value (the React component) and a type — the value wins
// at import time and turns `import type { Message }` into a runtime
// reference, which the TS build rejects.
interface AgentticMessage {
  id: string;
  role: 'user' | 'agent';
  content: Array<{
    type: 'text' | 'component' | 'context' | 'data';
    text?: string;
    component?: ComponentType<Record<string, unknown>>;
    componentProps?: Record<string, unknown>;
  }>;
  timestamp: number;
  archived: boolean;
  showIcon: boolean;
}

// Convert our wire-shape AskMessage[] into the Agenttic UI Message[]
// shape. Each turn becomes one Agenttic message; for assistant turns
// with refs or dispatched runs, a 'component' content block carrying
// AssistantChips is appended so chips render inside the bubble.
//
// Ids are derived from the array index — message order is append-only
// within a session, so the index is stable for any given turn.
export function adaptMessages(
  messages: AskMessage[],
  connection: Connection,
  onChipNavigated: () => void,
): AgentticMessage[] {
  return messages.map((msg, i) => {
    const content: AgentticMessage['content'] = [
      { type: 'text', text: msg.content || (msg.role === 'assistant' ? '_(no reply)_' : '') },
    ];

    if (msg.role === 'assistant') {
      const hasChips =
        (msg.references?.length ?? 0) > 0 ||
        (msg.dispatched?.length ?? 0) > 0;
      if (hasChips) {
        content.push({
          type: 'component',
          component: AssistantChips as never,
          componentProps: {
            references: msg.references,
            dispatched: msg.dispatched,
            connection,
            onChipNavigated,
          },
        });
      }
    }

    return {
      id: `m-${i}`,
      role: msg.role === 'assistant' ? 'agent' : 'user',
      content,
      timestamp: 0,
      archived: false,
      showIcon: false,
    };
  });
}
