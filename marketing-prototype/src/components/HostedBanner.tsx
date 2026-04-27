// On the hosted version (GHES Pages or any non-dev build) the Vite dev proxy
// doesn't exist, so the Approve & apply path will fail. This banner is the
// cheapest way to set the right expectation without disabling the button —
// teammates can still see and click through the full flow.

export default function HostedBanner() {
  if (import.meta.env.DEV) return null;
  return (
    <div
      className="text-xs text-center py-1.5 border-b"
      style={{
        background: '#FEFCE8',
        borderColor: '#FDE68A',
        color: '#713F12',
      }}
    >
      <span className="font-semibold">Read-only demo</span> — Approve & apply
      writes to the live store only when you run locally
      (<code className="font-mono">npm run dev</code>). Click through to see the
      flow; the apply step will throw on this hosted build.
    </div>
  );
}
