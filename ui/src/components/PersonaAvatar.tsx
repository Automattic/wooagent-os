// Small persona-colored avatar (the WPDS persona-color exception — see
// CLAUDE.md). Rendered in three places: the page eyebrow on Kanban, the
// bottom-right of each kanban card, and the W tile in the sidebar header.

export type PersonaKey = 'mk' | 'pr' | 'in' | 'ac' | 'rp' | 'ss' | 'cs';

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
