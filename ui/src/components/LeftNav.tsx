import { NavLink, useLocation } from 'react-router-dom';
import { Badge, Stack, Text } from '@wordpress/ui';
import {
  Icon,
  inbox,
  columns,
  people,
  category,
  box,
  store,
  shield,
  key,
} from '@wordpress/icons';

interface NavItem {
  to: string;
  label: string;
  icon: { type: string } | unknown;
  /** Render as a paused/dim non-clickable affordance (V2 placeholder). */
  paused?: boolean;
  /** Show this counter on the right side of the link. */
  badge?: number;
}

interface Props {
  marketingInReview: number;
  /** WooCommerce store hostname rendered in the connected-store footer. For
      now this is the daemon hostname — the daemon is the only store-context
      we surface. Will become a real store identifier once the daemon
      exposes one. */
  daemonHostname: string;
  /** When true, the sidebar is open in mobile drawer mode. Has no effect on
      desktop (the desktop layout is sticky-positioned via CSS). */
  isOpen?: boolean;
  /** Called when an item inside the sidebar is selected — used to close the
      drawer on mobile. */
  onItemClick?: () => void;
}

export default function LeftNav({
  marketingInReview,
  daemonHostname,
  isOpen = false,
  onItemClick,
}: Props) {
  const loc = useLocation();
  // Board is the "active" route while on the kanban or any issue detail.
  const onBoard = loc.pathname === '/' || loc.pathname.startsWith('/issues/');

  const inbox_items: NavItem[] = [
    { to: '/my-issues', label: 'My issues', icon: inbox, paused: true },
    {
      to: '/',
      label: 'Board',
      icon: columns,
      badge: marketingInReview > 0 ? marketingInReview : undefined,
    },
  ];
  const fleet_items: NavItem[] = [
    { to: '/agents', label: 'Agents', icon: people },
    { to: '/abilities', label: 'Abilities', icon: category },
    { to: '/runtimes', label: 'Runtimes', icon: box },
  ];
  const settings_items: NavItem[] = [
    { to: '/settings', label: 'Stores', icon: store },
    { to: '/guardrails', label: 'Guardrails', icon: shield },
    { to: '/secrets', label: 'Secrets', icon: key },
  ];

  return (
    <aside className={`wa-sidebar${isOpen ? ' is-open' : ''}`}>
      {/* Brand header. The W tile uses the marketing persona color since the
          marketing agent is the V1 product surface — same documented exception
          as the rest of the persona-color use sites. */}
      <div
        style={{
          padding:
            'var(--wpds-dimension-padding-md) var(--wpds-dimension-padding-md)',
        }}
      >
        <Stack direction="row" gap="sm" align="center">
          <div
            className="wa-persona-avatar wa-persona-avatar--md"
            style={{
              background: 'var(--wa-persona-mk-bg)',
              color: 'var(--wa-persona-mk-ink)',
              borderRadius: 'var(--wpds-border-radius-md)',
            }}
            aria-hidden="true"
          >
            W
          </div>
          {/* Wordmark inherits `color: #ffffff` from .wa-sidebar (see app.css). No inline color needed. */}
          <Text variant="heading-sm">
            WooAgent
          </Text>
        </Stack>
      </div>

      <nav
        style={{
          flex: 1,
          overflowY: 'auto',
          padding:
            '0 var(--wpds-dimension-padding-xs) var(--wpds-dimension-padding-md)',
        }}
      >
        <NavGroup label="Inbox" items={inbox_items} active={onBoard ? '/' : ''} onItemClick={onItemClick} />
        <NavGroup label="Fleet" items={fleet_items} active={loc.pathname.startsWith('/agents') ? '/agents' : loc.pathname} onItemClick={onItemClick} />
        <NavGroup label="Settings" items={settings_items} active={loc.pathname} onItemClick={onItemClick} />
      </nav>

      <div
        style={{
          padding: 'var(--wpds-dimension-padding-md)',
          borderTop:
            '1px solid rgba(255, 255, 255, 0.08)',
        }}
      >
        <div
          className="wa-eyebrow"
          style={{ marginBottom: 4 }}
        >
          Connected store
        </div>
        <div
          style={{
            fontSize: 'var(--wpds-typography-font-size-xs)',
            color: 'rgba(255, 255, 255, 0.85)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
          title={daemonHostname}
        >
          {daemonHostname}
        </div>
        <Stack direction="row" gap="xs" align="center" style={{ marginTop: 4 }}>
          {/* Lighter sage green than --wpds-color-fg-content-success — that
              token is #002900 (designed for text on light surfaces) and is
              effectively invisible against the dark sidebar bg. */}
          <span
            style={{
              height: 6,
              width: 6,
              borderRadius: '50%',
              background: 'var(--wpds-color-stroke-surface-success)',
              display: 'inline-block',
            }}
          />
          <span
            style={{
              fontSize: 'var(--wpds-typography-font-size-xs)',
              color: 'rgba(255, 255, 255, 0.6)',
            }}
          >
            Pressable staging
          </span>
        </Stack>
      </div>
    </aside>
  );
}

interface NavGroupProps {
  label: string;
  items: NavItem[];
  active: string;
  onItemClick?: () => void;
}

function NavGroup({ label, items, active, onItemClick }: NavGroupProps) {
  return (
    <div style={{ paddingTop: 'var(--wpds-dimension-padding-md)' }}>
      <div
        className="wa-eyebrow"
        style={{
          padding: '0 var(--wpds-dimension-padding-sm)',
          marginBottom: 'var(--wpds-dimension-gap-xs)',
        }}
      >
        {label}
      </div>
      {items.map((item) => {
        const isActive =
          item.paused === true
            ? false
            : item.to === active ||
              (item.to !== '/' && active.startsWith(item.to));

        const inner = (
          <>
            <Icon icon={item.icon as never} size={24} />
            <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {item.label}
            </span>
            {item.badge !== undefined && (
              <Badge intent="high" aria-label={`${item.badge} in review`}>
                {String(item.badge)}
              </Badge>
            )}
          </>
        );

        if (item.paused) {
          return (
            <span
              key={item.to}
              className="wa-navlink wa-navlink--paused"
              aria-disabled="true"
              title="Coming in V2"
            >
              {inner}
            </span>
          );
        }

        return (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            onClick={onItemClick}
            className={() =>
              `wa-navlink${isActive ? ' wa-navlink--active' : ''}`
            }
          >
            {inner}
          </NavLink>
        );
      })}
    </div>
  );
}
