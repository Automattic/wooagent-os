import type { AskReference, AskDispatched, Connection } from '../../api/client';
import ReferenceChip from './ReferenceChip';
import DispatchChip from './DispatchChip';

interface Props {
  references?: AskReference[];
  dispatched?: AskDispatched[];
  connection: Connection;
  onChipNavigated: () => void;
}

// AssistantChips renders the reference + dispatch pills that hang off
// an assistant turn. Designed to be embedded as a `type: 'component'`
// content block inside an Agenttic UI Message — that places the chips
// inside the assistant bubble where Agenttic's animations and layout
// keep them aligned with the prose.
export default function AssistantChips({
  references,
  dispatched,
  connection,
  onChipNavigated,
}: Props) {
  const hasRefs = (references?.length ?? 0) > 0;
  const hasDispatched = (dispatched?.length ?? 0) > 0;
  if (!hasRefs && !hasDispatched) return null;

  return (
    <div className="wa-chat-chips">
      {references?.map((ref) => (
        <ReferenceChip
          key={`${ref.kind}:${ref.id}`}
          reference={ref}
          onNavigated={onChipNavigated}
        />
      ))}
      {dispatched?.map((d) => (
        <DispatchChip
          key={d.run_id}
          dispatched={d}
          connection={connection}
          onNavigated={onChipNavigated}
        />
      ))}
    </div>
  );
}
