import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Notice } from '@wordpress/components';
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
      <main className="max-w-[900px] mx-auto px-6 py-12">
        <Notice status="warning" isDismissible={false}>
          That content task isn’t in the queue.{' '}
          <Link to="/" className="underline">
            Back to board
          </Link>
        </Notice>
      </main>
    );
  }

  const variant = task.variants.find((v) => v.id === selected) ?? task.variants[0];
  const newDescription = variant?.body ?? '';

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
        title: 'Couldn’t apply to store',
        body: e instanceof WooApiError ? e.message : String(e),
      });
    } finally {
      setApplying(false);
    }
  };

  return (
    <>
      <div className="bg-white border-b border-border">
        <div className="max-w-[1500px] mx-auto px-6 h-12 flex items-center gap-3">
          <Link to="/" className="text-xs text-muted hover:text-ink">
            ← Board
          </Link>
          <div className="h-4 w-px bg-border" />
          <span className="text-xs font-mono tabular text-muted">{task.id}</span>
          <StatusBadge status={task.status} />
          <KindBadge kind="content" />
        </div>
      </div>

      <main className="max-w-[1500px] mx-auto px-6 py-8">
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-2">
            <div className="h-6 w-6 rounded bg-mk-bg text-mk flex items-center justify-center text-[11px] font-bold">
              MK
            </div>
            <div className="text-xs text-muted">
              <span className="font-semibold text-ink">Marketing agent</span>{' '}
              proposes content
            </div>
            <div className="text-xs text-muted-2">·</div>
            <div className="text-xs tabular font-mono text-muted">
              {task.surfacedAt} · Claude Sonnet 4.6
            </div>
          </div>
          <h1
            className="display font-semibold"
            style={{ fontSize: 26, lineHeight: 1.25 }}
          >
            {task.title}
          </h1>
          <p className="text-sm mt-2 max-w-2xl text-ink-soft">
            Three voice variants. Pick one, approve, and the agent writes it
            straight to WooCommerce. The previous copy is snapshotted —
            reversible from the Done column.
          </p>
        </div>

        {/* Summary tiles */}
        <div className="grid grid-cols-4 gap-3 mb-6">
          <div className="card p-4">
            <div className="eyebrow">Scope</div>
            <div className="text-lg font-semibold mt-1 tabular display">
              {boundProduct?.name ?? task.productName ?? '1 product'}
            </div>
            <div className="text-xs tabular text-muted">
              {task.variants.length} variants generated
            </div>
          </div>
          <div className="card p-4">
            <div className="eyebrow">Brand voice match</div>
            <div className="flex items-center gap-2 mt-1">
              <div className="text-lg font-semibold tabular display">
                {variant?.voice ?? 0}%
              </div>
              <div className="flex-1 score-bar">
                <div
                  style={{
                    width: `${variant?.voice ?? 0}%`,
                    background: '#BE185D',
                  }}
                />
              </div>
            </div>
            <div className="text-xs text-muted">vs. your voice model</div>
          </div>
          <div className="card p-4">
            <div className="eyebrow">SEO score</div>
            <div className="flex items-center gap-2 mt-1">
              <div className="text-lg font-semibold tabular display">
                {variant?.seo ?? 0}
              </div>
              <div className="flex-1 score-bar">
                <div
                  style={{
                    width: `${variant?.seo ?? 0}%`,
                    background: '#16A34A',
                  }}
                />
              </div>
            </div>
            <div className="text-xs text-muted">Yoast · out of 100</div>
          </div>
          <div className="card p-4">
            <div className="eyebrow">Est. impact</div>
            <div
              className="text-lg font-semibold mt-1 tabular font-mono"
              style={{ color: '#16A34A' }}
            >
              +14% CTR
            </div>
            <div className="text-xs tabular text-muted">
              on product listing pages
            </div>
          </div>
        </div>

        <div
          className="grid"
          style={{ gridTemplateColumns: '1fr 300px', gap: 20 }}
        >
          <main>
            {/* Current description */}
            <div className="card mb-4 overflow-hidden">
              <div
                className="px-5 py-3 border-b border-border flex items-center justify-between"
                style={{ background: '#FAFAFA' }}
              >
                <div className="flex items-center gap-2">
                  <div className="eyebrow">Current description</div>
                  {boundProduct ? (
                    <span className="tag tag-info text-[10px]">
                      Live · ID {boundProduct.id}
                    </span>
                  ) : (
                    <span className="tag tag-warn text-[10px]">
                      No product bound
                    </span>
                  )}
                </div>
                <div className="text-xs tabular font-mono text-muted">
                  {boundProduct
                    ? `${stripHtml(boundProduct.description).length} chars · live`
                    : `${(task.currentDescription ?? '').length} chars · sample`}
                </div>
              </div>
              <div className="p-5">
                {loadErr && (
                  <Notice status="warning" isDismissible={false}>
                    Couldn’t fetch the live description: {loadErr}
                  </Notice>
                )}
                <p className="content-body text-muted">
                  {boundProduct
                    ? stripHtml(boundProduct.description) ||
                      '(empty — agent will fill this in)'
                    : task.currentDescription ?? ''}
                </p>
              </div>
            </div>

            {/* Variants */}
            <div className="flex items-center justify-between mb-3 mt-6">
              <div>
                <div className="eyebrow !text-mk">Proposed · Pick one</div>
                <div
                  className="display font-semibold mt-1"
                  style={{ fontSize: 16 }}
                >
                  Three variants · each with different emphasis
                </div>
              </div>
              <button className="btn btn-secondary text-xs" type="button">
                <svg
                  className="h-3.5 w-3.5"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M4 4v5h5M4 13a8 8 0 108-8v0"
                  />
                </svg>
                Regenerate
              </button>
            </div>

            {task.variants.map((v) => (
              <div
                key={v.id}
                onClick={() => setSelected(v.id)}
                className={`card variant-card ${
                  selected === v.id ? 'selected' : ''
                } ${v.recommended ? 'recommended' : ''} p-5 mb-3`}
              >
                <div className="flex items-start gap-3 mb-3">
                  <div
                    className={`h-6 w-6 rounded-full flex items-center justify-center text-xs font-bold tabular flex-none ${
                      selected === v.id
                        ? 'bg-primary text-white'
                        : 'border border-border-strong text-ink-soft'
                    }`}
                  >
                    {v.id}
                  </div>
                  <div className="flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="tag tag-mk text-[10px]">{v.label}</span>
                      <span
                        className={`tag text-[10px] ${
                          v.seo >= 80
                            ? 'tag-ok'
                            : v.seo >= 70
                              ? 'tag-warn'
                              : 'tag-err'
                        }`}
                      >
                        SEO · {v.seo}
                      </span>
                      <span
                        className={`tag text-[10px] ${
                          v.voice >= 90 ? 'tag-ok' : 'tag-warn'
                        }`}
                      >
                        Voice · {v.voice}%
                      </span>
                      <span className="tag tag-neutral text-[10px] tabular">
                        {v.charCount} chars
                      </span>
                    </div>
                  </div>
                </div>
                <p className="content-body">{v.body}</p>
                {v.note && (
                  <div className="mt-4 pt-4 border-t border-border text-xs text-muted">
                    {v.note}
                  </div>
                )}
              </div>
            ))}
          </main>

          <aside className="space-y-4">
            <div className="card p-5">
              <div className="flex items-center justify-between mb-3">
                <div className="eyebrow !text-mk">Brand voice check</div>
                <span className="tag tag-ok tabular text-[10px]">
                  {variant?.voice ?? 0}%
                </span>
              </div>
              <div className="space-y-2.5 text-xs text-ink-soft">
                <div className="flex items-start gap-2">
                  <span className="text-ok mt-0.5">✓</span>
                  <span>
                    Avoids{' '}
                    <span className="font-mono bg-err-bg text-err px-1 rounded">
                      luxe
                    </span>{' '}
                    and{' '}
                    <span className="font-mono bg-err-bg text-err px-1 rounded">
                      premium
                    </span>
                  </span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-ok mt-0.5">✓</span>
                  <span>
                    Uses preferred terms{' '}
                    <span className="font-mono bg-ok-bg text-ok px-1 rounded">
                      small-batch
                    </span>
                    ,{' '}
                    <span className="font-mono bg-ok-bg text-ok px-1 rounded">
                      handcrafted
                    </span>
                  </span>
                </div>
                <div className="flex items-start gap-2">
                  <span className="text-ok mt-0.5">✓</span>
                  <span>Tone lands between warm and sincere — your target</span>
                </div>
              </div>
            </div>

            <div className="card p-5">
              <div className="eyebrow mb-3">Yoast SEO breakdown</div>
              <div className="space-y-2 text-xs">
                {[
                  ['Keyphrase in first sentence', 'ok'],
                  ['Text length', 'ok'],
                  ['Readability', 'ok'],
                  ['Keyphrase density', 'warn'],
                  ['Passive voice', 'ok'],
                ].map(([label, level]) => (
                  <div
                    key={label}
                    className="flex items-center justify-between"
                  >
                    <span className="text-ink-soft">{label}</span>
                    <span
                      className={`h-2 w-2 rounded-full inline-block ${
                        level === 'ok' ? 'bg-ok' : 'bg-warn'
                      }`}
                    />
                  </div>
                ))}
              </div>
            </div>

            <div className="rounded-lg p-4 border bg-ok-bg border-ok-border">
              <div className="font-semibold text-xs mb-1 text-ok">
                Reversible · always
              </div>
              <div className="text-xs leading-relaxed" style={{ color: '#14532D' }}>
                The previous copy is snapshotted before write. Revert any
                product in one click from the Done column.
              </div>
            </div>
          </aside>
        </div>

        {/* Sticky action bar */}
        <div className="sticky bottom-6 mt-6 z-20">
          <div className="card p-3 flex items-center gap-3 shadow-sticky">
            <div className="flex items-center gap-2 pl-2 pr-4 border-r border-border">
              <div className="h-8 w-8 rounded-md flex items-center justify-center font-bold tabular bg-info-bg text-primary">
                {variant?.id ?? 'A'}
              </div>
              <div className="text-xs">
                <div className="font-medium">
                  Variant {variant?.id} selected ·{' '}
                  {boundProduct
                    ? `→ ${boundProduct.name}`
                    : 'no live product bound'}
                </div>
                <div className="text-muted">
                  Approval will write to WooCommerce via the dev proxy.
                </div>
              </div>
            </div>
            <button className="btn btn-ghost text-xs" type="button">
              Ask the agent
            </button>
            <button className="btn btn-danger text-xs" type="button">
              Reject all
            </button>
            <div className="flex-1" />
            <button
              className="btn btn-secondary text-xs"
              type="button"
              onClick={() => nav('/')}
              disabled={applying}
            >
              Cancel
            </button>
            <button
              className="btn btn-approve"
              type="button"
              onClick={onApprove}
              disabled={applying || task.status === 'done'}
            >
              {applying ? (
                'Applying…'
              ) : (
                <>
                  ✓ Approve & apply to store
                  <span className="kbd">⌘↵</span>
                </>
              )}
            </button>
          </div>
        </div>
      </main>
    </>
  );
}

function stripHtml(html: string): string {
  if (!html) return '';
  const tmp = document.createElement('div');
  tmp.innerHTML = html;
  return (tmp.textContent ?? tmp.innerText ?? '').trim();
}
