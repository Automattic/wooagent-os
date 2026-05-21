import type { AskMessage, Connection } from '../../api/client';
import ReferenceChip from './ReferenceChip';
import DispatchChip from './DispatchChip';

interface Props {
  message: AskMessage;
  connection: Connection;
  onChipNavigated: () => void;
}

// Message renders one chat turn. User turns are a right-aligned bubble
// with a soft brand-info background. Assistant turns are a full-width
// neutral surface that also hosts reference chips and dispatched
// receipts below the prose.
//
// CUSTOM: chat-bubble layout. (a) WPDS has no canonical chat-bubble
// primitive — Card is too heavy, Notice is for system messages, Stack
// alone doesn't carry the alignment + max-width semantics. (b) Two
// thin wrapper divs around the prose, the chips below for assistant
// turns. (c) Will collapse into a shared primitive if a second
// product surface (e.g. specialist chat in Phase 2) needs the same
// shape — current scope is just CoS chat.
export default function Message({ message, connection, onChipNavigated }: Props) {
  if (message.role === 'user') {
    return (
      <div className="wa-chat-msg wa-chat-msg--user">
        <div className="wa-chat-bubble wa-chat-bubble--user">
          {message.content}
        </div>
      </div>
    );
  }

  // Assistant turn — prose, then refs, then dispatched.
  return (
    <div className="wa-chat-msg wa-chat-msg--assistant">
      <div className="wa-chat-bubble wa-chat-bubble--assistant">
        {message.content || <em>(no reply)</em>}
      </div>
      {(message.references?.length ?? 0) > 0 && (
        <div className="wa-chat-chips">
          {message.references!.map((ref) => (
            <ReferenceChip
              key={`${ref.kind}:${ref.id}`}
              reference={ref}
              onNavigated={onChipNavigated}
            />
          ))}
        </div>
      )}
      {(message.dispatched?.length ?? 0) > 0 && (
        <div className="wa-chat-chips">
          {message.dispatched!.map((d) => (
            <DispatchChip
              key={d.run_id}
              dispatched={d}
              connection={connection}
              onNavigated={onChipNavigated}
            />
          ))}
        </div>
      )}
    </div>
  );
}
