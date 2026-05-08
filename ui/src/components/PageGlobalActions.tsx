import { useState } from 'react';
import { Button, SearchControl } from '@wordpress/components';
import { comment } from '@wordpress/icons';

interface Props {
  onAskAgent: () => void;
  showSearch?: boolean;
}

// Right-side actions shared across every WPDS <Page> in WooAgent: an optional
// global search input (stub for V1) and the "Ask agent" button. Each screen
// passes it via Page's `actions` prop so the page heading + search + Ask
// agent always sit on a single horizontal band — replacing the prior
// standalone TopBar component, which has been removed.
//
// Pages that render their own search via DataViews (Agents, Abilities) pass
// `showSearch={false}` to drop the redundant top-bar search.
export default function PageGlobalActions({ onAskAgent, showSearch = true }: Props) {
  const [query, setQuery] = useState('');
  // Wrap search + Ask agent in a single flex group so Page's actions slot
  // treats them as one item (sitting together on the right) rather than
  // splitting them across the available width with space-between.
  // `size="compact"` matches the @wordpress/dataviews in-table search; we
  // intentionally leave SearchControl with its WPDS default styling rather
  // than overriding internals.
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
      {showSearch && (
        <div style={{ width: 280 }}>
          {/* Default size (40px) matches the Button's __next40pxDefaultSize
              so Search and Ask agent share a baseline height. */}
          <SearchControl
            __nextHasNoMarginBottom
            value={query}
            onChange={setQuery}
            label="Search"
            placeholder="Search"
            hideLabelFromVision
          />
        </div>
      )}
      <Button
        __next40pxDefaultSize
        variant="secondary"
        icon={comment}
        onClick={onAskAgent}
      >
        Ask agent
      </Button>
    </div>
  );
}
