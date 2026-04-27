import { useEffect, useState } from 'react';
import { Notice, Spinner } from '@wordpress/components';
import { loadSettings, saveSettings, type StoreSettings } from '../lib/settings';
import { listProducts, WooApiError, type WooProduct } from '../lib/woo';
import { useApp } from '../App';

interface Props {
  open: boolean;
  onClose: () => void;
}

export default function SettingsDrawer({ open, onClose }: Props) {
  const { pushToast } = useApp();
  const [settings, setSettings] = useState<StoreSettings>(loadSettings());
  const [products, setProducts] = useState<WooProduct[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (open) setSettings(loadSettings());
  }, [open]);

  if (!open) return null;

  const onSave = () => {
    saveSettings(settings);
    pushToast({ kind: 'success', title: 'Settings saved' });
    onClose();
  };

  const onTest = async () => {
    setErr(null);
    setLoading(true);
    try {
      const list = await listProducts(settings);
      setProducts(list);
      if (list.length === 0) {
        setErr('Connected, but the store has no published products.');
      }
    } catch (e) {
      setErr(e instanceof WooApiError ? e.message : String(e));
      setProducts(null);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-40 flex justify-end bg-black/30"
      role="dialog"
      aria-modal="true"
      onClick={onClose}
    >
      <div
        className="w-[460px] max-w-full h-full bg-white shadow-xl overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="px-6 h-14 flex items-center justify-between border-b border-border sticky top-0 bg-white">
          <div className="font-semibold text-sm">Store settings</div>
          <button
            className="btn btn-ghost text-xs"
            onClick={onClose}
            aria-label="Close"
          >
            Close
          </button>
        </header>

        <div className="p-6 space-y-5">
          <div>
            <div className="eyebrow mb-1.5">Store</div>
            <div className="text-xs text-muted leading-relaxed">
              Live updates go through the Vite dev proxy → WooCommerce REST API.
              Set a different target in <code className="font-mono">.env.local</code>{' '}
              with <code className="font-mono">WOO_BASE=...</code>.
            </div>
            <input
              className="mt-2 w-full px-3 py-2 text-sm border border-border-strong rounded-md font-mono"
              value={settings.storeLabel}
              readOnly
            />
          </div>

          <div>
            <label className="eyebrow block mb-1.5">Consumer key</label>
            <input
              className="w-full px-3 py-2 text-sm border border-border-strong rounded-md font-mono"
              placeholder="ck_xxxxxxxxxxxxxxxx"
              value={settings.consumerKey}
              onChange={(e) =>
                setSettings({ ...settings, consumerKey: e.target.value.trim() })
              }
            />
          </div>

          <div>
            <label className="eyebrow block mb-1.5">Consumer secret</label>
            <input
              type="password"
              className="w-full px-3 py-2 text-sm border border-border-strong rounded-md font-mono"
              placeholder="cs_xxxxxxxxxxxxxxxx"
              value={settings.consumerSecret}
              onChange={(e) =>
                setSettings({
                  ...settings,
                  consumerSecret: e.target.value.trim(),
                })
              }
            />
            <div className="text-[11px] text-muted mt-1.5">
              Generate at{' '}
              <span className="font-mono">
                /wp-admin/admin.php?page=wc-settings&tab=advanced&section=keys
              </span>
              . Stored only in this browser.
            </div>
          </div>

          <div>
            <button
              className="btn btn-secondary text-xs"
              onClick={onTest}
              disabled={loading}
            >
              {loading ? (
                <>
                  <Spinner /> Testing…
                </>
              ) : (
                'Test connection'
              )}
            </button>
          </div>

          {err && (
            <Notice status="error" isDismissible={false}>
              {err}
            </Notice>
          )}

          {products && products.length > 0 && (
            <div>
              <div className="eyebrow mb-1.5">
                Bind demo to a published product
              </div>
              <div className="text-xs text-muted mb-2">
                The Content Review screen will load the current description for
                this product and apply approved variants to it.
              </div>
              <div className="card divide-y divide-border max-h-[280px] overflow-y-auto">
                {products.map((p) => (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() =>
                      setSettings({ ...settings, productId: p.id })
                    }
                    className={`w-full text-left px-3 py-2.5 hover:bg-neutral-50 flex items-center gap-2 ${
                      settings.productId === p.id ? 'bg-info-bg' : ''
                    }`}
                  >
                    <div className="flex-1 min-w-0">
                      <div className="text-xs font-medium truncate">
                        {p.name}
                      </div>
                      <div className="text-[10px] tabular font-mono text-muted">
                        ID {p.id} · {p.sku || 'no SKU'}
                      </div>
                    </div>
                    {settings.productId === p.id && (
                      <span className="tag tag-info text-[10px]">Bound</span>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        <footer className="px-6 py-4 border-t border-border sticky bottom-0 bg-white flex justify-end gap-2">
          <button className="btn btn-ghost text-xs" onClick={onClose}>
            Cancel
          </button>
          <button className="btn btn-primary text-xs" onClick={onSave}>
            Save
          </button>
        </footer>
      </div>
    </div>
  );
}
