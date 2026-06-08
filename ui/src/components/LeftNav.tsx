import { NavLink, useLocation } from 'react-router-dom';
import { Badge, Stack, Text } from '@wordpress/ui';
import { Button, Dropdown } from '@wordpress/components';
import {
  Icon,
  inbox,
  columns,
  archive,
  published,
  people,
  category,
  box,
  store,
  shield,
  key,
  cog,
  backup,
  arrowUpRight,
} from '@wordpress/icons';
import type { Connection, Store } from '../api/client';
import { relativeTime } from '../lib/boardItems';
import logoUrl from '../assets/wooagent-logo.png';

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
  /** Count of issues currently awaiting operator review, across all
      personas. Drives the badge on the Needs review nav entry. */
  inReviewCount: number;
  /** Which Inbox section should render as active when the operator is on
      a detail page. App computes this from the viewed issue/batch status so
      a dismissed item keeps "Archived" highlighted, a done item keeps
      "Done", and everything else stays on "Needs review." Null = let the
      path-based fallback (current behavior) decide. */
  activeInbox?: 'needs-review' | 'done' | 'archived' | null;
  /** Active daemon connection — drives the footer popover's URL + token
      preview. The hostname shown in the footer chip is derived from
      connection.daemonUrl. */
  connection: Connection;
  /** The paired Woo store, if any. Drives the footer chip's status dot,
      and the popover's "Open in WP-admin" link, skills count, and last-
      discovered timestamp. Null while still probing or pre-pairing. */
  store?: Store | null;
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
  inReviewCount,
  activeInbox = null,
  connection,
  store: pairedStore = null,
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
  // Status text for the footer chip. We only have free info from /v1/stores
  // — no store name yet — so we surface the pairing status itself. The
  // .mystagingwebsite.com hosting hint stays as a lightweight URL pattern
  // match (no daemon change required). Add more hosts here as needed.
  const isPaired = pairedStore?.status === 'paired';
  const storeStatusLabel = (() => {
    if (!pairedStore) return 'Not connected';
    if (pairedStore.status === 'pairing') return 'Pairing…';
    if (pairedStore.status === 'expired') return 'Pairing expired';
    if (pairedStore.status === 'failed') return 'Connection failed';
    // status === 'paired'
    try {
      const host = new URL(pairedStore.url).host;
      if (host.endsWith('.mystagingwebsite.com')) return 'Pressable staging';
      if (host.endsWith('.wpcomstaging.com')) return 'WordPress.com staging';
      if (host.endsWith('.wordpress.com')) return 'WordPress.com';
    } catch {
      /* fall through */
    }
    return 'Connected';
  })();
  const wpAdminUrl = pairedStore?.url
    ? `${pairedStore.url.replace(/\/$/, '')}/wp-admin/`
    : null;
  const loc = useLocation();
  // Inbox highlight resolution. App supplies `activeInbox` for detail pages
  // (looked up from the entity status: dismissed → archived, done → done,
  // else needs-review). When App has no answer yet — entity not loaded, or
  // we're on a non-detail route — fall back to the original pathname rule
  // (queue + drill-in paths land on Needs review).
  const detailInboxPath =
    activeInbox === 'archived'
      ? '/archived'
      : activeInbox === 'done'
        ? '/done'
        : activeInbox === 'needs-review'
          ? '/needs-review'
          : null;
  const onNeedsReview =
    detailInboxPath === null &&
    (loc.pathname === '/' ||
      loc.pathname === '/needs-review' ||
      loc.pathname.startsWith('/issues/') ||
      loc.pathname.startsWith('/batches/'));

  const inbox_items: NavItem[] = [
    { to: '/my-issues', label: 'My issues', icon: inbox, paused: true },
    {
      to: '/needs-review',
      label: 'Needs review',
      icon: columns,
      badge: inReviewCount > 0 ? inReviewCount : undefined,
    },
    { to: '/done', label: 'Done', icon: published },
    { to: '/archived', label: 'Archived', icon: archive },
  ];
  const fleet_items: NavItem[] = [
    { to: '/agents', label: 'Agents', icon: people },
    { to: '/skills', label: 'Skills', icon: category },
    { to: '/runtimes', label: 'Routines', icon: box },
    { to: '/models', label: 'Models', icon: cog },
  ];
  const activity_items: NavItem[] = [
    { to: '/runs', label: 'Runs', icon: backup },
  ];
  const settings_items: NavItem[] = [
    { to: '/stores', label: 'Stores', icon: store },
    { to: '/guardrails', label: 'Guardrails', icon: shield },
    { to: '/secrets', label: 'Secrets', icon: key },
  ];

  return (
    <aside className={`wa-sidebar${isOpen ? ' is-open' : ''}`}>
      {/* Brand header: horse-head badge logo + WooAgent wordmark. */}
      <div
        style={{
          padding:
            'var(--wpds-dimension-padding-md) var(--wpds-dimension-padding-md)',
        }}
      >
        <Stack direction="row" gap="sm" align="center">
          {/* CUSTOM: the brand mark is a raster logo asset (circular horse-head
              badge), not a WPDS component — there is no design-system component
              for a product logo. The PNG carries its own black circular badge,
              which reads against the #1e1e1e sidebar. Documented in DESIGN.md
              alongside the sidebar-surface exception. */}
          <img
            src={logoUrl}
            alt=""
            aria-hidden="true"
            style={{ width: 40, height: 40, objectFit: 'contain', display: 'block' }}
          />
          {/* Wordmark inherits `color: #ffffff` from .wa-sidebar (see app.css). No inline color needed.
              CUSTOM: fontWeight 700 is off-token — WPDS exposes no `bold` weight (only
              regular/medium). Used here to match the approved brand lockup, which is a
              700-weight wordmark. Brand-header exception only; documented in DESIGN.md
              under "Brand logo". Don't reuse this weight elsewhere — body/heading text
              stays on the WPDS weight tokens. */}
          <Text variant="heading-md" style={{ fontWeight: 700 }}>
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
        <NavGroup label="Inbox" items={inbox_items} active={detailInboxPath ?? (onNeedsReview ? '/needs-review' : loc.pathname)} onItemClick={onItemClick} />
        <NavGroup label="Fleet" items={fleet_items} active={loc.pathname.startsWith('/agents') ? '/agents' : loc.pathname} onItemClick={onItemClick} />
        <NavGroup label="Activity" items={activity_items} active={loc.pathname.startsWith('/runs') ? '/runs' : loc.pathname} onItemClick={onItemClick} />
        <NavGroup label="Settings" items={settings_items} active={loc.pathname} onItemClick={onItemClick} />
      </nav>

      <div
        style={{
          borderTop:
            '1px solid rgba(255, 255, 255, 0.08)',
        }}
      >
        <Dropdown
          popoverProps={{
            placement: 'top-start',
            // Floating UI's offset middleware accepts {mainAxis, crossAxis};
            // WPDS's type narrows it to number but passes the value straight
            // through, so the object form works at runtime. mainAxis = 8px
            // gap above the toggle, crossAxis = 8px shift inward from the
            // viewport's left edge.
            offset: { mainAxis: 8, crossAxis: 8 } as unknown as number,
          }}
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
                Connection
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
                    effectively invisible against the dark sidebar bg. Non-paired
                    states (pairing/expired/failed) read as a muted amber so the
                    chip stops over-promising "all good". */}
                <span
                  style={{
                    height: 6,
                    width: 6,
                    borderRadius: '50%',
                    background: isPaired
                      ? 'var(--wpds-color-stroke-surface-success)'
                      : 'var(--wpds-color-stroke-surface-warning)',
                    display: 'inline-block',
                  }}
                />
                <span
                  style={{
                    fontSize: 'var(--wpds-typography-font-size-xs)',
                    color: 'rgba(255, 255, 255, 0.6)',
                  }}
                >
                  {storeStatusLabel}
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
                {pairedStore && (
                  <Stack direction="column" gap="xs">
                    <Text variant="body-sm" style={FOOTER_MUTED}>STORE</Text>
                    <Text variant="body-md">{pairedStore.url}</Text>
                    {wpAdminUrl && (
                      <span style={{ fontSize: 'var(--wpds-typography-font-size-xs)' }}>
                        <Button
                          variant="link"
                          href={wpAdminUrl}
                          target="_blank"
                          rel="noreferrer noopener"
                        >
                          Open in WP-admin
                          <Icon
                            icon={arrowUpRight}
                            size={16}
                            style={{
                              verticalAlign: 'text-bottom',
                              marginInlineStart: 'var(--wpds-dimension-padding-xs)',
                            }}
                          />
                        </Button>
                      </span>
                    )}
                    {(pairedStore.ability_count !== undefined ||
                      pairedStore.last_discovered_at) && (
                      <Text variant="body-sm" style={FOOTER_MUTED}>
                        {pairedStore.ability_count !== undefined && (
                          <>
                            {pairedStore.ability_count} skill
                            {pairedStore.ability_count === 1 ? '' : 's'}
                          </>
                        )}
                        {pairedStore.ability_count !== undefined &&
                          pairedStore.last_discovered_at && <> · </>}
                        {pairedStore.last_discovered_at && (
                          <>synced {relativeTime(pairedStore.last_discovered_at)}</>
                        )}
                      </Text>
                    )}
                  </Stack>
                )}
                {/* Dev/standalone-only diagnostics. In embedded mode the
                    token comes from window.__WOOAGENT_TOKEN__ (no client
                    state) and the URL is always same-origin — these rows
                    are unactionable noise for end users. */}
                {!embedded && (
                  <>
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
                  </>
                )}
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
              <Badge
                intent="none"
                className="wa-nav-badge"
                aria-label={`${item.badge} in review`}
              >
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
