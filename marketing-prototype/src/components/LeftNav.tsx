import { NavLink, useLocation } from 'react-router-dom';
import { useApp } from '../App';

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
];

const AGENTS: AgentItem[] = [
  {
    to: '/agents/chief',
    label: 'Chief of Staff',
    badge: 'CS',
    badgeBg: 'linear-gradient(135deg,#6366F1,#8B5CF6)',
    badgeFg: '#FFFFFF',
    state: 'active',
  },
  {
    to: '/agents/marketing',
    label: 'Marketing',
    badge: 'MK',
    badgeBg: '#FCE7F3',
    badgeFg: '#BE185D',
    state: 'focus',
  },
  {
    to: '/agents/pricing',
    label: 'Pricing',
    badge: 'PR',
    badgeBg: '#DBEAFE',
    badgeFg: '#1D4ED8',
    state: 'paused',
  },
  {
    to: '/agents/inventory',
    label: 'Inventory',
    badge: 'IN',
    badgeBg: '#FEF3C7',
    badgeFg: '#B45309',
    state: 'paused',
  },
  {
    to: '/agents/accounting',
    label: 'Accounting',
    badge: 'AC',
    badgeBg: '#DCFCE7',
    badgeFg: '#15803D',
    state: 'paused',
  },
  {
    to: '/agents/reporting',
    label: 'Reporting',
    badge: 'RP',
    badgeBg: '#EDE9FE',
    badgeFg: '#6D28D9',
    state: 'paused',
  },
  {
    to: '/agents/sales-support',
    label: 'Sales Support',
    badge: 'SS',
    badgeBg: '#CCFBF1',
    badgeFg: '#0F766E',
    state: 'paused',
  },
];

interface Props {
  onOpenSettings: () => void;
}

export default function LeftNav({ onOpenSettings }: Props) {
  const loc = useLocation();
  const { tasks } = useApp();
  const marketingInReview = tasks.filter((t) => t.status === 'in_review').length;
  // Marketing agent is "where you are" whenever the operator is on a marketing
  // route — i.e. board, content review, campaign, email — since this prototype
  // is the marketing slice. Other routes only highlight when explicit.
  const onMarketing =
    loc.pathname === '/' ||
    loc.pathname.startsWith('/issues/') ||
    loc.pathname === '/agents/marketing';

  return (
    <aside
      className="h-screen sticky top-0 flex flex-col flex-none bg-white border-r border-border"
      style={{ width: 240 }}
    >
      {/* Brand */}
      <div className="px-4 py-4 border-b border-border">
        <div className="flex items-center gap-2">
          <div className="h-7 w-7 rounded-md flex items-center justify-center text-white text-xs font-bold bg-primary">
            W
          </div>
          <div className="font-semibold text-sm">WooAgent OS</div>
        </div>
      </div>

      {/* App section */}
      <nav className="px-2 py-3">
        <div className="eyebrow px-2 mb-1.5">App</div>
        {APP_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `flex items-center gap-2 px-2 py-1.5 rounded-md text-sm mb-0.5 transition ${
                isActive
                  ? 'bg-info-bg text-primary font-medium'
                  : 'text-ink-soft hover:bg-neutral-100'
              }`
            }
          >
            <span
              className="font-mono text-xs w-4 text-center flex-none"
              aria-hidden="true"
            >
              {item.glyph}
            </span>
            <span>{item.label}</span>
          </NavLink>
        ))}
        <button
          type="button"
          onClick={onOpenSettings}
          className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md text-sm mb-0.5 text-ink-soft hover:bg-neutral-100"
        >
          <span
            className="font-mono text-xs w-4 text-center flex-none"
            aria-hidden="true"
          >
            ⚙
          </span>
          <span>Store settings</span>
        </button>
      </nav>

      {/* Agents section */}
      <nav className="px-2 py-3 border-t border-border flex-1 overflow-y-auto">
        <div className="eyebrow px-2 mb-1.5 flex items-center justify-between">
          <span>Agents</span>
          <span
            className="font-mono tabular text-[10px]"
            style={{ color: '#A1A1AA' }}
          >
            7
          </span>
        </div>
        {AGENTS.map((a) => (
          <NavLink
            key={a.to}
            to={a.to}
            className={() => {
              const here =
                a.label === 'Marketing'
                  ? onMarketing
                  : loc.pathname === a.to;
              return `flex items-center gap-2 px-2 py-1.5 rounded-md text-sm mb-0.5 transition ${
                here
                  ? 'bg-info-bg text-primary font-medium'
                  : a.state === 'paused'
                    ? 'text-muted hover:bg-neutral-100'
                    : 'text-ink-soft hover:bg-neutral-100'
              }`;
            }}
          >
            <span
              className="h-6 w-6 rounded-md flex items-center justify-center font-bold tabular text-[10px] flex-none"
              style={{ background: a.badgeBg, color: a.badgeFg }}
            >
              {a.badge}
            </span>
            <span className="flex-1 truncate">{a.label}</span>
            {a.state === 'active' && (
              <span
                className="h-1.5 w-1.5 rounded-full bg-ok inline-block flex-none"
                aria-label="active"
              />
            )}
            {a.state === 'focus' && marketingInReview > 0 && (
              <span
                className="text-[10px] tabular font-mono flex-none"
                style={{ color: '#BE185D' }}
              >
                {marketingInReview} in review
              </span>
            )}
            {a.state === 'paused' && (
              <span
                className="text-[10px] tabular font-mono flex-none"
                style={{ color: '#A1A1AA' }}
              >
                paused
              </span>
            )}
          </NavLink>
        ))}
      </nav>

      {/* Footer */}
      <div className="px-4 py-3 border-t border-border">
        <div className="eyebrow mb-1">Connected store</div>
        <div className="text-xs font-mono tabular truncate text-ink-soft">
          woo-demo-store-99cc5c
        </div>
        <div className="flex items-center gap-1.5 mt-1">
          <span className="h-1.5 w-1.5 rounded-full bg-ok inline-block" />
          <span className="text-[11px] text-muted">Pressable staging</span>
        </div>
      </div>
    </aside>
  );
}
