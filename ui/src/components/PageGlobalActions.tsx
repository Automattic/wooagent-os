import { useState } from 'react';
import { Button, SearchControl } from '@wordpress/components';
import { Icon, comment } from '@wordpress/icons';

interface Props {
  onAskAgent: () => void;
}

// Right-side actions shared across every WPDS <Page> in WooAgent: a global
// search input (stub for V1) and the "Ask agent" button. Each screen passes
// it via Page's `actions` prop so the page heading + search + Ask agent
// always sit on a single horizontal band — replacing the prior standalone
// TopBar component, which has been removed.
export default function PageGlobalActions({ onAskAgent }: Props) {
  const [query, setQuery] = useState('');
  // Wrap search + Ask agent in a single flex group so Page's actions slot
  // treats them as one item (sitting together on the right) rather than
  // splitting them across the available width with space-between.
  // `size="compact"` matches the @wordpress/dataviews in-table search; we
  // intentionally leave SearchControl with its WPDS default styling rather
  // than overriding internals.
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
      <div style={{ width: 280 }}>
        <SearchControl
          __nextHasNoMarginBottom
          size="compact"
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
