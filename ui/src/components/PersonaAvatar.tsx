// Small persona avatar (the WPDS persona-color exception — see CLAUDE.md).
// Rendered in three places: the page eyebrow on Kanban, the bottom-right of
// each kanban card, and the W tile in the sidebar header. Renders the
// per-persona illustration when one exists in src/assets/avatar/; falls back
// to the colored-initials monogram for personas without artwork.

import marketingUrl from '../assets/avatar/marketing.png';
import pricingUrl from '../assets/avatar/pricing.png';
import inventoryUrl from '../assets/avatar/inventory.png';
import accountingUrl from '../assets/avatar/accounting.png';
import reportingUrl from '../assets/avatar/reporting.png';
import salesSupportUrl from '../assets/avatar/sales-support.png';
import chiefUrl from '../assets/avatar/chief.png';

export type PersonaKey = 'mk' | 'pr' | 'in' | 'ac' | 'rp' | 'ss' | 'cs';

const AVATAR_IMAGES: Partial<Record<PersonaKey, string>> = {
  mk: marketingUrl,
  pr: pricingUrl,
  in: inventoryUrl,
  ac: accountingUrl,
  rp: reportingUrl,
  ss: salesSupportUrl,
  cs: chiefUrl,
};

export interface PersonaAvatarProps {
  persona: PersonaKey;
  /** Initials shown inside the avatar. Defaults to the persona key uppercased. */
  label?: string;
  /** xs (14), sm (18), md (24). Default sm. */
  size?: 'xs' | 'sm' | 'md';
  /** Render as an empty colored dot (no text). */
  dotOnly?: boolean;
}

export function PersonaAvatar({
  persona,
  label,
  size = 'sm',
  dotOnly = false,
}: PersonaAvatarProps) {
  const imageUrl = AVATAR_IMAGES[persona];
  if (imageUrl && !dotOnly) {
    return (
      <img
        className={`wa-persona-avatar wa-persona-avatar--${size} wa-persona-avatar--image`}
        src={imageUrl}
        alt=""
        aria-hidden="true"
      />
    );
  }
  const initials = (label ?? persona).toUpperCase();
  return (
    <span
      className={`wa-persona-avatar wa-persona-avatar--${size}`}
      style={{
        background: `var(--wa-persona-${persona}-bg)`,
        color: `var(--wa-persona-${persona}-ink)`,
      }}
      aria-hidden="true"
    >
      {dotOnly ? '' : initials}
    </span>
  );
}

// Map an Issue.persona string to the short key used by the CSS variables.
export function personaKeyFrom(persona: string | undefined): PersonaKey {
  switch (persona) {
    case 'marketing':
      return 'mk';
    case 'pricing':
      return 'pr';
    case 'inventory':
      return 'in';
    case 'accounting':
      return 'ac';
    case 'reporting':
      return 'rp';
    case 'sales-support':
    case 'sales_support':
      return 'ss';
    case 'chief':
    case 'chief-of-staff':
      return 'cs';
    default:
      return 'mk';
  }
}
