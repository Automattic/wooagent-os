import { NavLink, useLocation } from 'react-router-dom';
import { Stack, Text } from '@wordpress/ui';

interface AppItem {
  to: string;
  label: string;
  glyph: string;
}

interface AgentItem {
  to: string;
  label: string;
  badge: string;
  badgeBg: string;
  badgeFg: string;
  state: 'active' | 'focus' | 'paused';
}

const APP_ITEMS: AppItem[] = [
  { to: '/', label: 'Board', glyph: '▦' },
  { to: '/activity', label: 'Activity', glyph: '◷' },
  { to: '/abilities', label: 'Abilities', glyph: '◆' },
  { to: '/settings', label: 'Store settings', glyph: '⚙' },
];

const AGENTS: AgentItem[] = [
  {
    to: '/agents/chief',
    label: 'Chief of Staff',
    badge: 'CS',
    badgeBg: 'var(--wa-persona-cs-bg)',
    badgeFg: 'var(--wa-persona-cs-ink)',
    state: 'active',
  },
  {
    to: '/agents/marketing',
    label: 'Marketing',
    badge: 'MK',
    badgeBg: 'var(--wa-persona-mk-bg)',
    badgeFg: 'var(--wa-persona-mk-ink)',
    state: 'focus',
  },
  {
    to: '/agents/pricing',
    label: 'Pricing',
    badge: 'PR',
    badgeBg: 'var(--wa-persona-pr-bg)',
    badgeFg: 'var(--wa-persona-pr-ink)',
    state: 'paused',
  },
  {
    to: '/agents/inventory',
    label: 'Inventory',
    badge: 'IN',
    badgeBg: 'var(--wa-persona-in-bg)',
    badgeFg: 'var(--wa-persona-in-ink)',
    state: 'paused',
  },
  {
    to: '/agents/accounting',
    label: 'Accounting',
    badge: 'AC',
    badgeBg: 'var(--wa-persona-ac-bg)',
    badgeFg: 'var(--wa-persona-ac-ink)',
    state: 'paused',
  },
  {
    to: '/agents/reporting',
    label: 'Reporting',
    badge: 'RP',
    badgeBg: 'var(--wa-persona-rp-bg)',
    badgeFg: 'var(--wa-persona-rp-ink)',
    state: 'paused',
  },
  {
    to: '/agents/sales-support',
    label: 'Sales Support',
    badge: 'SS',
    badgeBg: 'var(--wa-persona-ss-bg)',
    badgeFg: 'var(--wa-persona-ss-ink)',
    state: 'paused',
  },
];

interface Props {
  marketingInReview: number;
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
  const onMarketing =
    loc.pathname === '/' ||
    loc.pathname.startsWith('/issues/') ||
    loc.pathname === '/agents/marketing';

  return (
    <aside className={`wa-sidebar${isOpen ? ' is-open' : ''}`}>
      <div
        style={{
          padding: 'var(--wpds-dimension-padding-md)',
          borderBottom:
            'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
        }}
      >
        <Stack direction="row" gap="sm" align="center">
          <div
            style={{
              height: 28,
              width: 28,
              borderRadius: 'var(--wpds-border-radius-md)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#ffffff',
              fontSize: 'var(--wpds-typography-font-size-xs)',
              fontWeight: 700,
              background: 'var(--wpds-color-bg-interactive-brand-strong)',
            }}
          >
            W
          </div>
          <Text variant="heading-sm">WooAgent OS</Text>
        </Stack>
      </div>

      <nav
        style={{
          padding: 'var(--wpds-dimension-padding-sm) var(--wpds-dimension-padding-xs)',
        }}
      >
        <div
          className="wa-eyebrow"
          style={{
            padding: '0 var(--wpds-dimension-padding-sm)',
            marginBottom: 'var(--wpds-dimension-gap-xs)',
          }}
        >
          App
        </div>
        {APP_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            onClick={onItemClick}
            className={({ isActive }) =>
              `wa-navlink${isActive ? ' wa-navlink--active' : ''}`
            }
          >
            <span
              className="wa-mono"
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                width: 16,
                textAlign: 'center',
                flex: 'none',
              }}
              aria-hidden="true"
            >
              {item.glyph}
            </span>
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>

      <nav
        style={{
          padding: 'var(--wpds-dimension-padding-sm) var(--wpds-dimension-padding-xs)',
          borderTop:
            'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
          flex: 1,
          overflowY: 'auto',
        }}
      >
        <div
          className="wa-eyebrow"
          style={{
            padding: '0 var(--wpds-dimension-padding-sm)',
            marginBottom: 'var(--wpds-dimension-gap-xs)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <span>Agents</span>
          <span className="wa-mono" style={{ fontSize: 10 }}>
            7
          </span>
        </div>
        {AGENTS.map((a) => {
          const here = a.label === 'Marketing' ? onMarketing : loc.pathname === a.to;
          return (
            <NavLink
              key={a.to}
              to={a.to}
              onClick={onItemClick}
              className={() => {
                const cls = ['wa-navlink'];
                if (here) cls.push('wa-navlink--active');
                else if (a.state === 'paused') cls.push('wa-navlink--paused');
                return cls.join(' ');
              }}
            >
              <span
                style={{
                  height: 24,
                  width: 24,
                  borderRadius: 'var(--wpds-border-radius-md)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontWeight: 700,
                  fontSize: 10,
                  fontVariantNumeric: 'tabular-nums',
                  flex: 'none',
                  background: a.badgeBg,
                  color: a.badgeFg,
                }}
              >
                {a.badge}
              </span>
              <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {a.label}
              </span>
              {a.state === 'active' && (
                <span
                  aria-label="active"
                  style={{
                    height: 6,
                    width: 6,
                    borderRadius: '50%',
                    background: 'var(--wpds-color-fg-content-success)',
                    flex: 'none',
                  }}
                />
              )}
              {a.state === 'focus' && marketingInReview > 0 && (
                <span
                  className="wa-mono"
                  style={{
                    fontSize: 10,
                    color: 'var(--wa-persona-mk-ink)',
                    flex: 'none',
                  }}
                >
                  {marketingInReview} in review
                </span>
              )}
              {a.state === 'paused' && (
                <span
                  className="wa-mono"
                  style={{
                    fontSize: 10,
                    color: 'var(--wpds-color-fg-content-neutral-weak)',
                    flex: 'none',
                  }}
                >
                  paused
                </span>
              )}
            </NavLink>
          );
        })}
      </nav>

      <div
        style={{
          padding: 'var(--wpds-dimension-padding-md)',
          borderTop:
            'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
        }}
      >
        <div className="wa-eyebrow" style={{ marginBottom: 4 }}>
          Connected store
        </div>
        <div
          className="wa-mono"
          style={{
            fontSize: 'var(--wpds-typography-font-size-xs)',
            color: 'var(--wpds-color-fg-content-neutral)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
          }}
          title={daemonHostname}
        >
          {daemonHostname}
        </div>
        <Stack direction="row" gap="xs" align="center" style={{ marginTop: 4 }}>
          <span
            style={{
              height: 6,
              width: 6,
              borderRadius: '50%',
              background: 'var(--wpds-color-fg-content-success)',
              display: 'inline-block',
            }}
          />
          <span
            style={{
              fontSize: 11,
              color: 'var(--wpds-color-fg-content-neutral-weak)',
            }}
          >
            Daemon connected
          </span>
        </Stack>
      </div>
    </aside>
  );
}
