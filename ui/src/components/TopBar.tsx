import { Button, SearchControl } from '@wordpress/components';
import { Icon, comment } from '@wordpress/icons';
import { useState } from 'react';

interface Props {
  onAskAgent: () => void;
}

// Global top bar: search input (stub for V1) + Ask agent button.
export default function TopBar({ onAskAgent }: Props) {
  const [query, setQuery] = useState('');
  return (
    <div className="wa-topbar">
      <div style={{ flex: 1, maxWidth: 480 }}>
        <SearchControl
          __nextHasNoMarginBottom
          value={query}
          onChange={setQuery}
          label="Search"
          placeholder="Search"
          hideLabelFromVision
        />
      </div>
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
