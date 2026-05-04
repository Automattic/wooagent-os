import { Icon, search, comment } from '@wordpress/icons';
import { Button } from '@wordpress/components';

interface Props {
  onAskAgent: () => void;
}

// Global top bar: search input (stub for V1) + Ask agent button. Sticks at
// the top of the main content column. The Search has no behavior yet —
// rendered to match Figma so the layout shape is right when search lands.
export default function TopBar({ onAskAgent }: Props) {
  return (
    <div className="wa-topbar">
      <label className="wa-topbar__search">
        <Icon icon={search} size={18} />
        <input type="search" placeholder="Search" aria-label="Search" />
      </label>
      <Button
        variant="secondary"
        icon={<Icon icon={comment} size={16} />}
        onClick={onAskAgent}
      >
        Ask agent
      </Button>
    </div>
  );
}
