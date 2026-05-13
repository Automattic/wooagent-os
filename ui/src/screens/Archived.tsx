import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge, Stack, Text } from '@wordpress/ui';
import { Spinner } from '@wordpress/components';
import { Page } from '@wordpress/admin-ui';
import { api, type Connection, type Issue } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';
import PageGlobalActions from '../components/PageGlobalActions';

interface Props {
  connection: Connection;
  onAskAgent: () => void;
}

// Archive page lists issues in the terminal "dismissed" state (or the older
// "rejected" state — daemon hasn't renamed yet). Built as a simple table-style
// list for now; the DataViews migration that aligns with the Agents + Abilities
// screens is tracked separately and will replace this scaffold.
export default function Archived({ connection, onAskAgent }: Props) {
  const nav = useNavigate();
  const [issues, setIssues] = useState<Issue[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await api.issues(connection);
        if (cancelled) return;
        setIssues(res.issues);
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [connection]);

  const dismissed = useMemo(() => {
    if (!issues) return null;
    return issues
      .filter((i) => i.status === 'dismissed' || i.status === 'rejected')
      .sort((a, b) => {
        const aT = a.dismissed_at ?? a.updated_at;
        const bT = b.dismissed_at ?? b.updated_at;
        return bT.localeCompare(aT);
      });
  }, [issues]);

  return (
    <Page
      title="Archive"
      hasPadding
      actions={<PageGlobalActions onAskAgent={onAskAgent} />}
    >
      {error && (
        <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-error)' }}>
          {error}
        </Text>
      )}

      {!dismissed ? (
        <Stack direction="row" justify="center" align="center" style={{ padding: 'var(--wpds-dimension-padding-2xl)' }}>
          <Spinner />
        </Stack>
      ) : dismissed.length === 0 ? (
        <Stack direction="column" gap="sm" align="center" style={{ padding: 'var(--wpds-dimension-padding-3xl) 0' }}>
          <Text variant="heading-md">No dismissed proposals yet</Text>
          <Text
            variant="body-sm"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              textAlign: 'center',
              maxWidth: '420px',
              textWrap: 'pretty',
            }}
          >
            When you dismiss a proposal from the board, it'll appear here.
            Dismissed proposals are kept for 30 days before they're permanently
            deleted.
          </Text>
        </Stack>
      ) : (
        <Stack direction="column" gap="md">
          <Stack direction="row" justify="space-between" align="center">
            <span /> {/* spacer left for future filter chips */}
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              {dismissed.length} archived
            </Text>
          </Stack>

          {/* CUSTOM: table-style list of dismissed issues. (a) DataViews would
              be the canonical WPDS pattern, but is scoped as a separate
              follow-up to align with Agents/Abilities. (b) Grid header +
              bordered rows mimicking the Figma. (c) Replace with DataViews
              per follow-up. */}
          <div role="table" aria-label="Archived proposals" className="wa-archive-table">
            <div role="row" className="wa-archive-table__head">
              <span role="columnheader">Proposal</span>
              <span role="columnheader">Type</span>
              <span role="columnheader">Agent</span>
              <span role="columnheader">Dismissed</span>
              <span role="columnheader">Reason</span>
            </div>
            {dismissed.map((issue) => {
              const personaKey = personaKeyFrom(issue.persona);
              return (
                <button
                  key={issue.id}
                  type="button"
                  role="row"
                  onClick={() => nav(`/issues/${issue.id}`)}
                  className="wa-archive-row"
                >
                  <span role="cell" className="wa-archive-row__title">
                    <Text variant="body-md" style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}>
                      {issue.title}
                    </Text>
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                    >
                      {issue.id}
                    </Text>
                  </span>
                  <span role="cell">
                    <KindBadgeForArchive persona={personaKey} />
                  </span>
                  <span role="cell" className="wa-archive-row__agent">
                    {personaKey && <PersonaAvatar persona={personaKey} size="sm" />}
                    <Text variant="body-sm">{issue.persona ?? '—'}</Text>
                  </span>
                  <span role="cell">
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                    >
                      {formatDismissedAt(issue.dismissed_at ?? issue.updated_at)}
                    </Text>
                  </span>
                  <span role="cell">
                    {issue.dismiss_reason ? (
                      <Badge intent="none">
                        {formatReason(issue.dismiss_reason)}
                      </Badge>
                    ) : (
                      <Text
                        variant="body-sm"
                        style={{
                          color: 'var(--wpds-color-fg-content-neutral-weak)',
                          fontStyle: 'italic',
                        }}
                      >
                        No reason given
                      </Text>
                    )}
                  </span>
                </button>
              );
            })}
          </div>
        </Stack>
      )}
    </Page>
  );
}

function KindBadgeForArchive({ persona }: { persona: string | null }) {
  // Persona-mapped kind label, matching the canonical KindBadge convention
  // (Content / Campaign / Pricing / Email). Marketing → Content default; this
  // is a placeholder while the daemon doesn't expose issue kind explicitly.
  if (persona === 'marketing') return <Badge intent="none">Content</Badge>;
  if (persona === 'pricing') return <Badge intent="none">Pricing</Badge>;
  if (persona === 'sales_support') return <Badge intent="none">Email</Badge>;
  return <Badge intent="none">Proposal</Badge>;
}

function formatDismissedAt(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const now = new Date();
  const isToday = date.toDateString() === now.toDateString();
  if (isToday) {
    return `Today at ${date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}`;
  }
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (date.toDateString() === yesterday.toDateString()) {
    return 'Yesterday';
  }
  const days = Math.floor((now.getTime() - date.getTime()) / 86_400_000);
  if (days < 7) return `${days} days ago`;
  if (days < 14) return '1 week ago';
  return `${Math.floor(days / 7)} weeks ago`;
}

function formatReason(reason: string): string {
  // Map canonical reason keys back to the chip labels used in the Dismiss
  // dialog. Keep the map in sync with the DismissReason union in api/client.ts.
  const map: Record<string, string> = {
    tone_off: 'Tone is off',
    wrong_product_focus: 'Wrong product focus',
    not_needed_now: 'Not needed now',
    write_myself: "I'll write this myself",
    wrong_timing: 'Wrong timing',
    out_of_stock: 'Out of stock',
    price_too_aggressive: 'Price too aggressive',
    needs_brand_review: 'Needs brand review',
    will_handle_myself: 'Will handle myself',
    other: 'Other',
  };
  return map[reason] ?? reason;
}
