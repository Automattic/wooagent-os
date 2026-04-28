import { useEffect, useState } from 'react';
import { Stack, Text } from '@wordpress/ui';
import { Button, Notice, Spinner, TextControl } from '@wordpress/components';
import { loadSettings, saveSettings, type StoreSettings } from '../lib/settings';
import { listProducts, WooApiError, type WooProduct } from '../lib/woo';
import { useApp } from '../App';

interface Props {
  open: boolean;
  onClose: () => void;
}

// A side-anchored drawer. WPDS Modal is a centered dialog, so we render the
// drawer ourselves with WPDS tokens — same shape as the daemon UI's right
// rail, just scrolling within a fixed-position panel.
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
      role="dialog"
      aria-modal="true"
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 40,
        display: 'flex',
        justifyContent: 'flex-end',
        background: 'rgba(0,0,0,0.3)',
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          width: 460,
          maxWidth: '100%',
          height: '100%',
          background: 'var(--wpds-color-bg-surface-neutral)',
          boxShadow: 'var(--wpds-elevation-lg)',
          overflowY: 'auto',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <header
          style={{
            padding: '0 var(--wpds-dimension-padding-lg)',
            height: 56,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            borderBottom:
              'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
            position: 'sticky',
            top: 0,
            background: 'var(--wpds-color-bg-surface-neutral)',
          }}
        >
          <Text variant="heading-sm">Store settings</Text>
          <Button variant="tertiary" onClick={onClose} aria-label="Close">
            Close
          </Button>
        </header>

        <div
          style={{
            padding: 'var(--wpds-dimension-padding-lg)',
            flex: 1,
          }}
        >
          <Stack direction="column" gap="lg">
            <div>
              <span className="wa-eyebrow">Store</span>
              <Text
                variant="body-sm"
                style={{
                  marginTop: 6,
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                }}
              >
                Live updates go through the Vite dev proxy → WooCommerce REST
                API. Set a different target in{' '}
                <code className="wa-mono">.env.local</code> with{' '}
                <code className="wa-mono">WOO_BASE=...</code>.
              </Text>
              <div style={{ marginTop: 8 }}>
                <TextControl
                  __next40pxDefaultSize
                  label=""
                  hideLabelFromVision
                  value={settings.storeLabel}
                  readOnly
                  onChange={() => {}}
                />
              </div>
            </div>

            <div>
              <TextControl
                __next40pxDefaultSize
                label="Consumer key"
                placeholder="ck_xxxxxxxxxxxxxxxx"
                value={settings.consumerKey}
                onChange={(v) =>
                  setSettings({ ...settings, consumerKey: v.trim() })
                }
              />
            </div>

            <div>
              <TextControl
                __next40pxDefaultSize
                type="password"
                label="Consumer secret"
                placeholder="cs_xxxxxxxxxxxxxxxx"
                value={settings.consumerSecret}
                onChange={(v) =>
                  setSettings({ ...settings, consumerSecret: v.trim() })
                }
              />
              <Text
                variant="body-sm"
                style={{
                  marginTop: 6,
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                  fontSize: 11,
                }}
              >
                Generate at{' '}
                <code className="wa-mono">
                  /wp-admin/admin.php?page=wc-settings&tab=advanced&section=keys
                </code>
                . Stored only in this browser.
              </Text>
            </div>

            <div>
              <Button variant="secondary" onClick={onTest} disabled={loading}>
                {loading ? (
                  <>
                    <Spinner /> Testing…
                  </>
                ) : (
                  'Test connection'
                )}
              </Button>
            </div>

            {err && (
              <Notice status="error" isDismissible={false}>
                {err}
              </Notice>
            )}

            {products && products.length > 0 && (
              <div>
                <span className="wa-eyebrow">Bind demo to a published product</span>
                <Text
                  variant="body-sm"
                  style={{
                    marginTop: 6,
                    marginBottom: 8,
                    color: 'var(--wpds-color-fg-content-neutral-weak)',
                  }}
                >
                  The Content Review screen will load the current description
                  for this product and apply approved variants to it.
                </Text>
                <div
                  style={{
                    border:
                      'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                    borderRadius: 'var(--wpds-border-radius-md)',
                    background: 'var(--wpds-color-bg-surface-neutral)',
                    maxHeight: 280,
                    overflowY: 'auto',
                  }}
                >
                  {products.map((p, i) => (
                    <button
                      key={p.id}
                      type="button"
                      onClick={() =>
                        setSettings({ ...settings, productId: p.id })
                      }
                      style={{
                        width: '100%',
                        textAlign: 'left',
                        padding:
                          'var(--wpds-dimension-padding-sm) var(--wpds-dimension-padding-md)',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 'var(--wpds-dimension-gap-sm)',
                        background:
                          settings.productId === p.id
                            ? 'var(--wpds-color-bg-surface-info-weak)'
                            : 'transparent',
                        border: 'none',
                        borderBottom:
                          i === products.length - 1
                            ? 'none'
                            : 'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
                        cursor: 'var(--wpds-cursor-control)',
                      }}
                    >
                      <Stack direction="column" gap="xs" style={{ flex: 1, minWidth: 0 }}>
                        <Text
                          variant="body-sm"
                          style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                        >
                          {p.name}
                        </Text>
                        <span
                          className="wa-mono"
                          style={{
                            fontSize: 10,
                            color: 'var(--wpds-color-fg-content-neutral-weak)',
                          }}
                        >
                          ID {p.id} · {p.sku || 'no SKU'}
                        </span>
                      </Stack>
                      {settings.productId === p.id && (
                        <span
                          style={{
                            fontSize: 10,
                            padding: '2px 8px',
                            borderRadius: 'var(--wpds-border-radius-sm)',
                            background: 'var(--wpds-color-bg-surface-info-weak)',
                            color: 'var(--wpds-color-fg-interactive-brand)',
                          }}
                        >
                          Bound
                        </span>
                      )}
                    </button>
                  ))}
                </div>
              </div>
            )}
          </Stack>
        </div>

        <footer
          style={{
            padding:
              'var(--wpds-dimension-padding-md) var(--wpds-dimension-padding-lg)',
            borderTop:
              'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-neutral-weak)',
            position: 'sticky',
            bottom: 0,
            background: 'var(--wpds-color-bg-surface-neutral)',
            display: 'flex',
            justifyContent: 'flex-end',
            gap: 'var(--wpds-dimension-gap-sm)',
          }}
        >
          <Button variant="tertiary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={onSave}>
            Save
          </Button>
        </footer>
      </div>
    </div>
  );
}
