import { Card, Stack, Text } from '@wordpress/ui';
import { useApp } from '../App';

// We render our own stack instead of @wordpress/components Snackbar because
// Snackbar surfaces a single message at a time; this prototype shows a
// queue with explicit dismiss buttons.
export default function ToastStack() {
  const { toasts, dismissToast } = useApp();
  if (toasts.length === 0) return null;
  return (
    <div
      style={{
        position: 'fixed',
        bottom: 24,
        right: 24,
        zIndex: 50,
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--wpds-dimension-gap-sm)',
        width: 360,
      }}
    >
      {toasts.map((t) => {
        const accentBorder =
          t.kind === 'success'
            ? 'var(--wpds-color-stroke-surface-success)'
            : t.kind === 'error'
              ? 'var(--wpds-color-stroke-surface-error)'
              : 'var(--wpds-color-stroke-surface-info)';
        const dotBg =
          t.kind === 'success'
            ? 'var(--wpds-color-bg-interactive-brand-strong)'
            : t.kind === 'error'
              ? 'var(--wpds-color-bg-interactive-error-strong)'
              : 'var(--wpds-color-bg-interactive-brand-strong)';
        return (
          <Card.Root
            key={t.id}
            style={{ borderColor: accentBorder, boxShadow: 'var(--wpds-elevation-md)' }}
          >
            <Card.Content>
              <Stack direction="row" gap="sm" align="start">
                <span
                  style={{
                    marginTop: 2,
                    height: 20,
                    width: 20,
                    borderRadius: '50%',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#ffffff',
                    fontSize: 11,
                    fontWeight: 700,
                    flex: 'none',
                    background: dotBg,
                  }}
                >
                  {t.kind === 'success' ? '✓' : t.kind === 'error' ? '!' : 'i'}
                </span>
                <Stack direction="column" gap="xs" style={{ flex: 1, minWidth: 0 }}>
                  <Text
                    variant="body-sm"
                    style={{ fontWeight: 'var(--wpds-typography-font-weight-medium)' }}
                  >
                    {t.title}
                  </Text>
                  {t.body && (
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
                    >
                      {t.body}
                    </Text>
                  )}
                </Stack>
                <button
                  type="button"
                  onClick={() => dismissToast(t.id)}
                  aria-label="Dismiss"
                  style={{
                    background: 'transparent',
                    border: 'none',
                    cursor: 'var(--wpds-cursor-control)',
                    color: 'var(--wpds-color-fg-content-neutral-weak)',
                    fontSize: 'var(--wpds-typography-font-size-xs)',
                    flex: 'none',
                  }}
                >
                  ✕
                </button>
              </Stack>
            </Card.Content>
          </Card.Root>
        );
      })}
    </div>
  );
}
