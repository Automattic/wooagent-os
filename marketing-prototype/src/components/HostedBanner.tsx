// On the hosted version (GHES Pages or any non-dev build) the Vite dev proxy
// doesn't exist, so the Approve & apply path will fail. This banner sets the
// right expectation without disabling the button — teammates can still see
// and click through the full flow.

export default function HostedBanner() {
  if (import.meta.env.DEV) return null;
  return (
    <div
      style={{
        fontSize: 'var(--wpds-typography-font-size-xs)',
        textAlign: 'center',
        padding: 'var(--wpds-dimension-padding-xs)',
        background: 'var(--wpds-color-bg-surface-warning-weak)',
        borderBottom:
          'var(--wpds-border-width-sm) solid var(--wpds-color-stroke-surface-warning)',
        color: 'var(--wpds-color-fg-content-warning)',
      }}
    >
      <strong>Read-only demo</strong> — Approve & apply writes to the live store
      only when you run locally (
      <code style={{ fontFamily: 'var(--wpds-typography-font-family-mono)' }}>
        npm run dev
      </code>
      ). Click through to see the flow; the apply step will throw on this hosted
      build.
    </div>
  );
}
