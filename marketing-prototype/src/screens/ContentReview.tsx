import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Card, Stack, Text } from '@wordpress/ui';
import { Button, Notice } from '@wordpress/components';
import { useApp } from '../App';
import type { ContentTask } from '../data/types';
import { loadSettings } from '../lib/settings';
import {
  getProduct,
  updateProductDescription,
  WooApiError,
  type WooProduct,
} from '../lib/woo';
import { KindBadge, StatusBadge } from '../components/StatusBadge';

export default function ContentReview() {
  const { id } = useParams<{ id: string }>();
  const { tasks, setTaskStatus, pushToast } = useApp();
  const nav = useNavigate();

  const task = useMemo(
    () => tasks.find((t): t is ContentTask => t.id === id && t.kind === 'content'),
    [tasks, id],
  );

  const [selected, setSelected] = useState<'A' | 'B' | 'C'>(
    task?.variants.find((v) => v.recommended)?.id ?? 'A',
  );
  const [boundProduct, setBoundProduct] = useState<WooProduct | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [applying, setApplying] = useState(false);
  const settings = loadSettings();

  useEffect(() => {
    if (!settings.productId || !settings.consumerKey) return;
    let cancelled = false;
    getProduct(settings, settings.productId)
      .then((p) => {
        if (!cancelled) setBoundProduct(p);
      })
      .catch((e) => {
        if (!cancelled)
          setLoadErr(e instanceof WooApiError ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (!task) {
    return (
      <main
        style={{
          maxWidth: 900,
          margin: '0 auto',
          padding: 'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
        }}
      >
        <Notice status="warning" isDismissible={false}>
          That content task isn't in the queue.{' '}
          <Link to="/" style={{ textDecoration: 'underline' }}>
            Back to board
          </Link>
        </Notice>
      </main>
    );
  }

  const variant = task.variants.find((v) => v.id === selected) ?? task.variants[0];
  const newDescription = variant?.body ?? '';
  const reviewable = task.status !== 'done';

  const onApprove = async () => {
    const s = loadSettings();
    if (!s.productId || !s.consumerKey || !s.consumerSecret) {
      pushToast({
        kind: 'error',
        title: 'Bind a product first',
        body: 'Open Store settings, test the connection, and pick the product to apply this description to.',
      });
      return;
    }
    setApplying(true);
    try {
      const updated = await updateProductDescription(
        s,
        s.productId,
        newDescription,
      );
      setBoundProduct(updated);
      setTaskStatus(task.id, 'done');
      pushToast({
        kind: 'success',
        title: 'Live · description updated',
        body: `Variant ${variant?.id} written to "${updated.name}". Snapshot saved — revert from Done column.`,
      });
      nav('/');
    } catch (e) {
      pushToast({
        kind: 'error',
        title: "Couldn't apply to store",
        body: e instanceof WooApiError ? e.message : String(e),
      });
    } finally {
      setApplying(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
      {/* Breadcrumb header */}
      <div
        style={{
          background: 'var(--wpds-color-bg-surface-neutral)',
          borderBottom:
            'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
        }}
      >
        <div
          style={{
            maxWidth: 1500,
            margin: '0 auto',
            padding: '0 var(--wa-page-pad-x)',
            height: 48,
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--wpds-dimension-gap-sm)',
          }}
        >
          <Link
            to="/"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              fontSize: 'var(--wpds-typography-font-size-xs)',
            }}
          >
            ← Board
          </Link>
          <span
            style={{
              height: 16,
              width: 1,
              background: 'var(--wpds-color-stroke-surface-neutral)',
            }}
          />
          <span
            className="wa-mono"
            style={{
              fontSize: 'var(--wpds-typography-font-size-xs)',
              color: 'var(--wpds-color-fg-content-neutral-weak)',
            }}
          >
            {task.id}
          </span>
          <StatusBadge status={task.status} />
          <KindBadge kind="content" />
        </div>
      </div>

      <main
        style={{
          flex: 1,
          maxWidth: 1500,
          width: '100%',
          margin: '0 auto',
          padding: 'var(--wpds-dimension-padding-2xl) var(--wa-page-pad-x)',
        }}
      >
        {/* Persona eyebrow + title */}
        <Stack direction="column" gap="sm" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
          <Stack direction="row" gap="sm" align="center">
            <span
              style={{
                height: 24,
                width: 24,
                borderRadius: 'var(--wpds-border-radius-md)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontWeight: 700,
                fontSize: 11,
                background: 'var(--wa-persona-mk-bg)',
                color: 'var(--wa-persona-mk-ink)',
                flex: 'none',
              }}
            >
              MK
            </span>
            <Text
              variant="body-sm"
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
            >
              <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
                Marketing agent
              </strong>{' '}
              proposes content
            </Text>
            <span
              style={{ color: 'var(--wpds-color-fg-content-neutral-weak)', fontSize: 'var(--wpds-typography-font-size-xs)' }}
            >
              ·
            </span>
            <span
              className="wa-mono"
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                color: 'var(--wpds-color-fg-content-neutral-weak)',
              }}
            >
              {task.surfacedAt} · Claude Sonnet 4.6
            </span>
          </Stack>
          <Text
            variant="heading-2xl"
            render={<h1 style={{ margin: 0 }} />}
          >
            {task.title}
          </Text>
          <Text
            variant="body-sm"
            style={{ maxWidth: 760, color: 'var(--wpds-color-fg-content-neutral-weak)' }}
          >
            Three voice variants. Pick one, approve, and the agent writes it
            straight to WooCommerce. The previous copy is snapshotted —
            reversible from the Done column.
          </Text>
        </Stack>

        {/* KPI row */}
        <div className="wa-kpi-row" style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}>
          <Card.Root style={{ flex: 1, minWidth: 0 }}>
            <Card.Content>
              <Stack direction="column" gap="xs">
                <span className="wa-eyebrow">Scope</span>
                <Text
                  variant="heading-md"
                  style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                >
                  {boundProduct?.name ?? task.productName ?? '1 product'}
                </Text>
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                >
                  {task.variants.length} variants generated
                </Text>
              </Stack>
            </Card.Content>
          </Card.Root>

          <Card.Root style={{ flex: 1, minWidth: 0 }}>
            <Card.Content>
              <Stack direction="column" gap="xs">
                <span className="wa-eyebrow">Brand voice match</span>
                <Stack direction="row" gap="sm" align="center">
                  <Text
                    variant="heading-md"
                    style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                  >
                    {variant?.voice ?? 0}%
                  </Text>
                  <div className="wa-score-bar" style={{ flex: 1 }}>
                    <div
                      style={{
                        width: `${variant?.voice ?? 0}%`,
                        background: 'var(--wa-persona-mk-ink)',
                      }}
                    />
                  </div>
                </Stack>
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                >
                  vs. your voice model
                </Text>
              </Stack>
            </Card.Content>
          </Card.Root>

          <Card.Root style={{ flex: 1, minWidth: 0 }}>
            <Card.Content>
              <Stack direction="column" gap="xs">
                <span className="wa-eyebrow">SEO score</span>
                <Stack direction="row" gap="sm" align="center">
                  <Text
                    variant="heading-md"
                    style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                  >
                    {variant?.seo ?? 0}
                  </Text>
                  <div className="wa-score-bar" style={{ flex: 1 }}>
                    <div
                      style={{
                        width: `${variant?.seo ?? 0}%`,
                        background: 'var(--wpds-color-bg-interactive-success-strong, var(--wpds-color-fg-content-success))',
                      }}
                    />
                  </div>
                </Stack>
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                >
                  Yoast · out of 100
                </Text>
              </Stack>
            </Card.Content>
          </Card.Root>

          <Card.Root style={{ flex: 1, minWidth: 0 }}>
            <Card.Content>
              <Stack direction="column" gap="xs">
                <span className="wa-eyebrow">Est. impact</span>
                <Text
                  variant="heading-md"
                  style={{
                    color: 'var(--wpds-color-fg-content-success)',
                    fontFamily: 'var(--wpds-typography-font-family-mono)',
                    fontWeight: 'var(--wpds-typography-font-weight-medium)',
                  }}
                >
                  +14% CTR
                </Text>
                <Text
                  variant="body-sm"
                  style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                >
                  on product listing pages
                </Text>
              </Stack>
            </Card.Content>
          </Card.Root>
        </div>

        {/* Body grid: variants + sidebar */}
        <div className="wa-detail-body">
          <div className="wa-detail-main">
            {/* Current description */}
            <Card.Root>
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <Stack direction="row" gap="sm" align="center">
                    <span className="wa-eyebrow">Current description</span>
                    {boundProduct ? (
                      <span
                        style={{
                          fontSize: 10,
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-surface-info-weak)',
                          color: 'var(--wpds-color-fg-interactive-brand)',
                        }}
                      >
                        Live · ID {boundProduct.id}
                      </span>
                    ) : (
                      <span
                        style={{
                          fontSize: 10,
                          padding: '2px 8px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                          background: 'var(--wpds-color-bg-surface-warning-weak)',
                          color: 'var(--wpds-color-fg-content-warning)',
                        }}
                      >
                        No product bound
                      </span>
                    )}
                  </Stack>
                  <span
                    className="wa-mono"
                    style={{
                      fontSize: 'var(--wpds-typography-font-size-xs)',
                      color: 'var(--wpds-color-fg-content-neutral-weak)',
                    }}
                  >
                    {boundProduct
                      ? `${stripHtml(boundProduct.description).length} chars · live`
                      : `${(task.currentDescription ?? '').length} chars · sample`}
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                {loadErr && (
                  <Notice status="warning" isDismissible={false}>
                    Couldn't fetch the live description: {loadErr}
                  </Notice>
                )}
                <Text
                  variant="body-md"
                  style={{
                    color: 'var(--wpds-color-fg-content-neutral-weak)',
                    lineHeight: 1.65,
                  }}
                >
                  {boundProduct
                    ? stripHtml(boundProduct.description) || '(empty — agent will fill this in)'
                    : task.currentDescription ?? ''}
                </Text>
              </Card.Content>
            </Card.Root>

            {/* Variants header */}
            <Stack direction="row" justify="space-between" align="end" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
              <Stack direction="column" gap="xs">
                <span className="wa-eyebrow wa-eyebrow--persona">Proposed · pick one</span>
                <Text variant="heading-md">
                  Three variants · each with different emphasis
                </Text>
              </Stack>
              <Button variant="secondary" type="button">
                ⟲ Regenerate
              </Button>
            </Stack>

            {/* Variant cards */}
            {task.variants.map((v) => (
              <button
                key={v.id}
                type="button"
                onClick={() => setSelected(v.id)}
                style={{
                  display: 'block',
                  width: '100%',
                  minWidth: 0,
                  textAlign: 'left',
                  padding: 0,
                  background: 'transparent',
                  border: 'none',
                }}
              >
                <Card.Root
                  className={`wa-variant-card${
                    selected === v.id ? ' wa-variant-card--selected' : ''
                  }`}
                >
                  <Card.Content>
                    {v.recommended && <span className="wa-agent-pick">Agent pick</span>}
                    <Stack direction="row" gap="md" align="start" style={{ marginBottom: 'var(--wpds-dimension-gap-sm)', minWidth: 0 }}>
                      <span
                        style={{
                          height: 24,
                          width: 24,
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontWeight: 700,
                          fontSize: 12,
                          flex: 'none',
                          background:
                            selected === v.id
                              ? 'var(--wpds-color-bg-interactive-brand-strong)'
                              : 'transparent',
                          color:
                            selected === v.id
                              ? '#ffffff'
                              : 'var(--wpds-color-fg-content-neutral)',
                          border:
                            selected === v.id
                              ? 'none'
                              : 'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-strong)',
                        }}
                      >
                        {v.id}
                      </span>
                      <Stack direction="row" gap="xs" wrap="wrap" style={{ flex: 1, minWidth: 0 }}>
                        <span
                          style={{
                            fontSize: 10,
                            padding: '2px 8px',
                            borderRadius: 'var(--wpds-border-radius-sm)',
                            background: 'var(--wa-persona-mk-bg)',
                            color: 'var(--wa-persona-mk-ink)',
                          }}
                        >
                          {v.label}
                        </span>
                        <span
                          style={{
                            fontSize: 10,
                            padding: '2px 8px',
                            borderRadius: 'var(--wpds-border-radius-sm)',
                            background:
                              v.seo >= 80
                                ? 'var(--wpds-color-bg-surface-success-weak)'
                                : v.seo >= 70
                                  ? 'var(--wpds-color-bg-surface-warning-weak)'
                                  : 'var(--wpds-color-bg-surface-error-weak)',
                            color:
                              v.seo >= 80
                                ? 'var(--wpds-color-fg-content-success)'
                                : v.seo >= 70
                                  ? 'var(--wpds-color-fg-content-warning)'
                                  : 'var(--wpds-color-fg-content-error)',
                          }}
                        >
                          SEO · {v.seo}
                        </span>
                        <span
                          style={{
                            fontSize: 10,
                            padding: '2px 8px',
                            borderRadius: 'var(--wpds-border-radius-sm)',
                            background:
                              v.voice >= 90
                                ? 'var(--wpds-color-bg-surface-success-weak)'
                                : 'var(--wpds-color-bg-surface-warning-weak)',
                            color:
                              v.voice >= 90
                                ? 'var(--wpds-color-fg-content-success)'
                                : 'var(--wpds-color-fg-content-warning)',
                          }}
                        >
                          Voice · {v.voice}%
                        </span>
                        <span
                          className="wa-mono"
                          style={{
                            fontSize: 10,
                            padding: '2px 8px',
                            borderRadius: 'var(--wpds-border-radius-sm)',
                            background: 'var(--wpds-color-bg-surface-neutral-strong)',
                            color: 'var(--wpds-color-fg-content-neutral-weak)',
                          }}
                        >
                          {v.charCount} chars
                        </span>
                      </Stack>
                    </Stack>
                    <Text variant="body-md" style={{ lineHeight: 1.65 }}>
                      {v.body}
                    </Text>
                    {v.note && (
                      <div
                        style={{
                          marginTop: 'var(--wpds-dimension-gap-md)',
                          paddingTop: 'var(--wpds-dimension-padding-md)',
                          borderTop:
                            'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                          fontSize: 'var(--wpds-typography-font-size-xs)',
                          color: 'var(--wpds-color-fg-content-neutral-weak)',
                        }}
                      >
                        {v.note}
                      </div>
                    )}
                  </Card.Content>
                </Card.Root>
              </button>
            ))}
          </div>

          {/* Sidebar rail */}
          <div className="wa-detail-rail">
            <Card.Root>
              <Card.Header>
                <Stack direction="row" justify="space-between" align="center">
                  <span className="wa-eyebrow wa-eyebrow--persona">
                    Brand voice check
                  </span>
                  <span
                    className="wa-mono"
                    style={{
                      fontSize: 10,
                      color: 'var(--wpds-color-fg-content-success)',
                    }}
                  >
                    {variant?.voice ?? 0}%
                  </span>
                </Stack>
              </Card.Header>
              <Card.Content>
                <Stack direction="column" gap="sm">
                  <div className="wa-check-row">
                    <span className="wa-check-mark">✓</span>
                    <Text variant="body-sm">
                      Avoids{' '}
                      <code
                        style={{
                          fontFamily: 'var(--wpds-typography-font-family-mono)',
                          background: 'var(--wpds-color-bg-surface-error-weak)',
                          color: 'var(--wpds-color-fg-content-error)',
                          padding: '1px 6px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                        }}
                      >
                        luxe
                      </code>{' '}
                      and{' '}
                      <code
                        style={{
                          fontFamily: 'var(--wpds-typography-font-family-mono)',
                          background: 'var(--wpds-color-bg-surface-error-weak)',
                          color: 'var(--wpds-color-fg-content-error)',
                          padding: '1px 6px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                        }}
                      >
                        premium
                      </code>
                    </Text>
                  </div>
                  <div className="wa-check-row">
                    <span className="wa-check-mark">✓</span>
                    <Text variant="body-sm">
                      Uses preferred terms{' '}
                      <code
                        style={{
                          fontFamily: 'var(--wpds-typography-font-family-mono)',
                          background: 'var(--wpds-color-bg-surface-success-weak)',
                          color: 'var(--wpds-color-fg-content-success)',
                          padding: '1px 6px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                        }}
                      >
                        small-batch
                      </code>
                      ,{' '}
                      <code
                        style={{
                          fontFamily: 'var(--wpds-typography-font-family-mono)',
                          background: 'var(--wpds-color-bg-surface-success-weak)',
                          color: 'var(--wpds-color-fg-content-success)',
                          padding: '1px 6px',
                          borderRadius: 'var(--wpds-border-radius-sm)',
                        }}
                      >
                        handcrafted
                      </code>
                    </Text>
                  </div>
                  <div className="wa-check-row">
                    <span className="wa-check-mark">✓</span>
                    <Text variant="body-sm">
                      Tone lands between warm and sincere — your target
                    </Text>
                  </div>
                </Stack>
              </Card.Content>
            </Card.Root>

            <Card.Root>
              <Card.Header>
                <span className="wa-eyebrow">Yoast SEO breakdown</span>
              </Card.Header>
              <Card.Content>
                <Stack direction="column" gap="sm">
                  {(
                    [
                      ['Keyphrase in first sentence', 'ok'],
                      ['Text length', 'ok'],
                      ['Readability', 'ok'],
                      ['Keyphrase density', 'warn'],
                      ['Passive voice', 'ok'],
                    ] as const
                  ).map(([label, level]) => (
                    <Stack key={label} direction="row" justify="space-between" align="center">
                      <Text variant="body-sm">{label}</Text>
                      <span
                        style={{
                          height: 8,
                          width: 8,
                          borderRadius: '50%',
                          display: 'inline-block',
                          background:
                            level === 'ok'
                              ? 'var(--wpds-color-fg-content-success)'
                              : 'var(--wpds-color-fg-content-caution)',
                        }}
                      />
                    </Stack>
                  ))}
                </Stack>
              </Card.Content>
            </Card.Root>

            <Card.Root
              style={{
                background: 'var(--wpds-color-bg-surface-success-weak)',
                borderColor: 'var(--wpds-color-stroke-surface-success)',
              }}
            >
              <Card.Content>
                <Stack direction="column" gap="xs">
                  <Text
                    variant="body-sm"
                    style={{
                      fontWeight: 'var(--wpds-typography-font-weight-medium)',
                      color: 'var(--wpds-color-fg-content-success)',
                    }}
                  >
                    Reversible · always
                  </Text>
                  <Text variant="body-sm" style={{ color: 'var(--wpds-color-fg-content-success-weak)' }}>
                    The previous copy is snapshotted before write. Revert any
                    product in one click from the Done column.
                  </Text>
                </Stack>
              </Card.Content>
            </Card.Root>
          </div>
        </div>
      </main>

      {/* Sticky action bar */}
      <div className="wa-action-bar">
        <div className="wa-action-bar-row">
          <Stack direction="row" gap="sm" align="center">
            <span
              style={{
                height: 28,
                width: 28,
                borderRadius: 'var(--wpds-border-radius-md)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontWeight: 700,
                fontSize: 12,
                background: 'var(--wpds-color-bg-surface-info-weak)',
                color: 'var(--wpds-color-fg-interactive-brand)',
                flex: 'none',
              }}
            >
              {variant?.id ?? 'A'}
            </span>
            <Stack direction="column" gap="xs">
              <Text
                variant="body-sm"
                style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
              >
                Variant {variant?.id} selected ·{' '}
                {boundProduct ? `→ ${boundProduct.name}` : 'no live product bound'}
              </Text>
              <Text
                variant="body-sm"
                style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
              >
                Approval will write to WooCommerce via the dev proxy.
              </Text>
            </Stack>
          </Stack>
          <div className="wa-action-bar-actions">
            <Button variant="tertiary" disabled>
              Ask the agent
            </Button>
            <Button variant="tertiary" isDestructive>
              Reject all
            </Button>
            <Button variant="tertiary" onClick={() => nav('/')} disabled={applying}>
              Cancel
            </Button>
            <Button
              variant="primary"
              onClick={onApprove}
              disabled={applying || !reviewable}
            >
              {applying ? 'Applying…' : '✓ Approve & apply to store'}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

function stripHtml(html: string): string {
  if (!html) return '';
  const tmp = document.createElement('div');
  tmp.innerHTML = html;
  return (tmp.textContent ?? tmp.innerText ?? '').trim();
}
