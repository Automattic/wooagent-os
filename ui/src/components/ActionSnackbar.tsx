import { Snackbar } from '@wordpress/components';

export interface SnackbarAction {
  label: string;
  onClick: () => void;
}

interface Props {
  text: string;
  /** Optional follow-up action rendered inline (e.g. Undo). */
  action?: SnackbarAction;
  onRemove: () => void;
  /** 'above-action-bar' floats the snackbar just above a page's fixed action
   *  bar (proposal / batch canvas); 'bottom' (default) sits at the bottom of
   *  the content canvas (board, roster). */
  placement?: 'bottom' | 'above-action-bar';
}

// Post-action confirmation snackbar — the same WPDS Snackbar the agent
// roster fires on a completed run. Wrapped in the shared `.wa-snackbar-host`
// (fixed, bottom-center). The host sets no width, so the snackbar sizes to
// its content rather than the fixed width shown in the Figma mockup.
// Auto-dismisses on the WPDS default timeout; the operator can also dismiss
// manually or trigger the inline action.
export default function ActionSnackbar({
  text,
  action,
  onRemove,
  placement = 'bottom',
}: Props) {
  const hostClass =
    placement === 'above-action-bar'
      ? 'wa-snackbar-host wa-snackbar-host--above-action-bar'
      : 'wa-snackbar-host';
  return (
    <div className={hostClass}>
      <Snackbar
        onRemove={onRemove}
        actions={action ? [{ label: action.label, onClick: action.onClick }] : undefined}
      >
        {text}
      </Snackbar>
    </div>
  );
}
