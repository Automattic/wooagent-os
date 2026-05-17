import { useState } from 'react';
import { Icon, box } from '@wordpress/icons';
import { personaKeyFrom } from './PersonaAvatar';
// Per-persona placeholder illustrations (Streamline Plump Color set,
// CC BY 4.0 — https://www.streamlinehq.com/icons/plump-color). Each SVG
// is templated with `var(--persona-mid)` / `var(--persona-ink)` so the
// tile binds the duotone fills to its own color tokens (fixed warm-gray
// by default). Imported as raw strings + inlined via dangerouslySetInnerHTML
// so the CSS variables actually cascade in (an <img src> sandbox wouldn't
// inherit them). See docs/specs/2026-05-16-product-images-design.md.
import customerSupportSvg from '../assets/illustrations/customer-support-3.svg?raw';
import pencilSquareSvg from '../assets/illustrations/pencil-square.svg?raw';
import tagAltSvg from '../assets/illustrations/tag-alt.svg?raw';
import archiveBoxSvg from '../assets/illustrations/archive-box.svg?raw';
import graphBarSvg from '../assets/illustrations/graph-bar-increase.svg?raw';
import bookSvg from '../assets/illustrations/book-1.svg?raw';
import userMultipleSvg from '../assets/illustrations/user-multiple-accounts.svg?raw';

export type ProductThumbnailSize = 'sm' | 'md' | 'lg';

interface Props {
  src?: string;
  alt?: string;
  /** Persona slug. Drives the placeholder illustration + tile color when
   *  src is missing or the <img> load fails. */
  persona?: string;
  size: ProductThumbnailSize;
}

const PERSONA_ILLUSTRATION: Record<string, string> = {
  marketing: pencilSquareSvg,
  pricing: tagAltSvg,
  'sales-support': customerSupportSvg,
  sales_support: customerSupportSvg,
  inventory: archiveBoxSvg,
  accounting: bookSvg,
  reporting: graphBarSvg,
  chief: userMultipleSvg,
  'chief-of-staff': userMultipleSvg,
};

// Pixel size of the FALLBACK Icon (used when the persona doesn't map to
// an illustration). Real illustrations size themselves via CSS to ~42%
// of the tile in the DataViews grid, full size in the fixed sm/md/lg
// rules — see .wa-product-thumb--placeholder svg in app.css.
const ICON_PX: Record<ProductThumbnailSize, number> = {
  sm: 18,
  md: 32,
  lg: 40,
};

export default function ProductThumbnail({ src, alt, persona, size }: Props) {
  const [failed, setFailed] = useState(false);
  const showImage = !!src && !failed;
  const iconSize = ICON_PX[size];

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

  const illustration = persona ? PERSONA_ILLUSTRATION[persona] : undefined;

  if (illustration) {
    // The persona key drives the tile-bg via the existing --wa-persona-*-bg
    // tokens (the documented persona-color exception — see CLAUDE.md).
    // Chief's token is a saturated gradient rather than a soft solid, so
    // we fall back to the neutral tile bg for Chief to keep the queue's
    // visual rhythm consistent.
    const personaKey = personaKeyFrom(persona);
    const tileBg =
      personaKey === 'cs'
        ? undefined
        : `var(--wa-persona-${personaKey}-bg)`;
    return (
      <div
        className={`wa-product-thumb wa-product-thumb--${size} wa-product-thumb--placeholder wa-product-thumb--illus`}
        aria-hidden="true"
        style={tileBg ? { background: tileBg } : undefined}
        dangerouslySetInnerHTML={{ __html: illustration }}
      />
    );
  }

  // Fallback for un-mapped personas: the legacy @wordpress/icons box glyph
  // on the neutral tile. Same treatment as before the illustration set
  // landed, so any future persona without a custom illustration still
  // renders something.
  return (
    <div
      className={`wa-product-thumb wa-product-thumb--${size} wa-product-thumb--placeholder`}
      aria-hidden="true"
    >
      <Icon icon={box} size={iconSize} />
    </div>
  );
}
