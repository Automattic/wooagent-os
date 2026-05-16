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

const PIXEL: Record<ProductThumbnailSize, { box: number; icon: number }> = {
  sm: { box: 40, icon: 18 },
  md: { box: 72, icon: 32 },
  lg: { box: 86, icon: 40 },
};

export default function ProductThumbnail({ src, alt, persona, size }: Props) {
  const [failed, setFailed] = useState(false);
  const showImage = !!src && !failed;
  const dims = PIXEL[size];
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
      <Icon icon={icon} size={dims.icon} />
    </div>
  );
}
