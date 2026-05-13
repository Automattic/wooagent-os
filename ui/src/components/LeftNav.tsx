import { NavLink, useLocation } from 'react-router-dom';
import { Badge, Stack, Text } from '@wordpress/ui';
import { Button, Dropdown } from '@wordpress/components';
import {
  Icon,
  inbox,
  columns,
  archive,
  people,
  category,
  box,
  store,
  shield,
  key,
  cog,
} from '@wordpress/icons';
import type { Connection } from '../api/client';

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
  /** Active daemon connection — drives the footer popover's URL + token
      preview. The hostname shown in the footer chip is derived from
      connection.daemonUrl. */
  connection: Connection;
  /** True when the UI is served by the local daemon (no browser-stored
      bearer). The footer popover hides the "Forget this connection"
      button in that mode since there's no client state to clear. */
  embedded: boolean;
  /** Clears the stored bearer + connection state. Invoked from the footer
      popover's "Forget this connection" button. */
  onForgetConnection: () => void;
  /** When true, the sidebar is open in mobile drawer mode. Has no effect on
      desktop (the desktop layout is sticky-positioned via CSS). */
  isOpen?: boolean;
  /** Called when an item inside the sidebar is selected — used to close the
      drawer on mobile. */
  onItemClick?: () => void;
}

const FOOTER_MUTED = {
  color: 'var(--wpds-color-fg-content-neutral-weak)',
} as const;

export default function LeftNav({
  marketingInReview,
  connection,
  embedded,
  onForgetConnection,
  isOpen = false,
  onItemClick,
}: Props) {
  const daemonHostname = (() => {
    try {
      return new URL(connection.daemonUrl).host;
    } catch {
      return connection.daemonUrl;
    }
  })();
  const tokenPreview = `${connection.token.slice(0, 12)}…`;
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
    { to: '/archived', label: 'Archived', icon: archive },
  ];
  const fleet_items: NavItem[] = [
    { to: '/agents', label: 'Agents', icon: people },
    { to: '/abilities', label: 'Skills', icon: category },
    { to: '/runtimes', label: 'Routines', icon: box },
    { to: '/models', label: 'Models', icon: cog },
  ];
  const settings_items: NavItem[] = [
    { to: '/stores', label: 'Stores', icon: store },
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
          {/* Brand tile uses WPDS interactive-brand-strong (indigo) per the
              i3.2 Figma. Was previously the marketing-persona pink as part
              of the persona-color exception, but moved to brand indigo so
              the persona-color exception stays scoped to identity surfaces
              (PersonaAvatar + KindBadge) only. */}
          <div
            className="wa-persona-avatar wa-persona-avatar--md"
            style={{
              background: 'var(--wpds-color-bg-interactive-brand-strong)',
              color: 'var(--wpds-color-fg-interactive-brand-strong)',
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
        <NavGroup label="Inbox" items={inbox_items} active={onBoard ? '/' : loc.pathname} onItemClick={onItemClick} />
        <NavGroup label="Fleet" items={fleet_items} active={loc.pathname.startsWith('/agents') ? '/agents' : loc.pathname} onItemClick={onItemClick} />
        <NavGroup label="Settings" items={settings_items} active={loc.pathname} onItemClick={onItemClick} />
      </nav>

      <div
        style={{
          borderTop:
            '1px solid rgba(255, 255, 255, 0.08)',
        }}
      >
        <Dropdown
          popoverProps={{ placement: 'top-start' }}
          renderToggle={({ isOpen: ddOpen, onToggle }) => (
            // CUSTOM: footer toggle is a full-width button styled to match
            // the dark sidebar surface. (a) WPDS Button doesn't expose a
            // dark-surface variant that fits inline footer chrome at this
            // weight. (b) Native <button> with `wa-sidebar-footer-toggle`
            // styles for hover/focus parity. (c) Follow-up: replace with
            // a WPDS Button variant if/when one ships for dark surfaces.
            <button
              type="button"
              aria-expanded={ddOpen}
              aria-haspopup="dialog"
              aria-label="Show connection details"
              onClick={onToggle}
              className="wa-sidebar-footer-toggle"
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
            </button>
          )}
          renderContent={({ onClose }) => (
            <div
              style={{
                minWidth: 280,
                padding: 'var(--wpds-dimension-padding-md)',
              }}
            >
              <Stack direction="column" gap="md">
                <Text variant="heading-sm">Connection</Text>
                <Stack direction="column" gap="xs">
                  <Text variant="body-sm" style={FOOTER_MUTED}>WOOAGENT URL</Text>
                  <Text variant="body-md" className="wa-mono">
                    {connection.daemonUrl}
                  </Text>
                </Stack>
                <Stack direction="column" gap="xs">
                  <Text variant="body-sm" style={FOOTER_MUTED}>TOKEN</Text>
                  <Text variant="body-md" className="wa-mono">
                    {tokenPreview}
                  </Text>
                </Stack>
                {embedded ? (
                  <Text variant="body-sm" style={FOOTER_MUTED}>
                    This UI is served by the local WooAgent daemon. Stop{' '}
                    <code>wooagent run</code> in your terminal to disconnect.
                  </Text>
                ) : (
                  <Stack direction="row">
                    <Button
                      variant="secondary"
                      __next40pxDefaultSize
                      onClick={() => {
                        onClose();
                        onForgetConnection();
                      }}
                    >
                      Forget this connection
                    </Button>
                  </Stack>
                )}
              </Stack>
            </div>
          )}
        />
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
