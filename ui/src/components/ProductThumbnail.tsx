import { useState } from 'react';
import { Icon } from '@wordpress/icons';
import {
  pencil,
  currencyDollar,
  comment,
  archive,
  pages,
  chartBar,
  people,
  box,
} from '@wordpress/icons';

export type ProductThumbnailSize = 'sm' | 'md' | 'lg';

interface Props {
  src?: string;
  alt?: string;
  /** Persona slug. Drives the placeholder icon when src is missing or
   *  the <img> load fails. */
  persona?: string;
  size: ProductThumbnailSize;
}

// Per-persona placeholder icons. Distinct glyph per agent so cards
// without product images still carry an at-a-glance signal of what
// the proposal is about. Future personas inherit the default `box`.
// See docs/specs/2026-05-16-product-images-design.md.
const PERSONA_ICON: Record<string, typeof box> = {
  marketing: pencil,
  pricing: currencyDollar,
  'sales-support': comment,
  inventory: archive,
  accounting: pages,
  reporting: chartBar,
  chief: people,
};

// Pixel size of the placeholder icon at each thumbnail size. The box
// dimensions (40 / 72 / 86) are owned by CSS (.wa-product-thumb--sm/md/lg)
// — the icon size is the one knob the component sets at render time.
const ICON_PX: Record<ProductThumbnailSize, number> = {
  sm: 18,
  md: 32,
  lg: 40,
};

export default function ProductThumbnail({ src, alt, persona, size }: Props) {
  const [failed, setFailed] = useState(false);
  const showImage = !!src && !failed;
  const iconSize = ICON_PX[size];
  const icon = (persona && PERSONA_ICON[persona]) || box;

  if (showImage) {
    return (
      <img
        src={src}
        alt={alt ?? ''}
        loading="lazy"
        onError={() => setFailed(true)}
        className={`wa-product-thumb wa-product-thumb--${size}`}
      />
    );
  }
  return (
    <div
      className={`wa-product-thumb wa-product-thumb--${size} wa-product-thumb--placeholder`}
      aria-hidden="true"
    >
      <Icon icon={icon} size={iconSize} />
    </div>
  );
}
