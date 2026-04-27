import { useApp } from '../App';

export default function ToastStack() {
  const { toasts, dismissToast } = useApp();
  if (toasts.length === 0) return null;
  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col gap-2 w-[360px]">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={`card p-3 flex gap-3 items-start shadow-lg ${
            t.kind === 'success'
              ? 'border-ok-border'
              : t.kind === 'error'
                ? 'border-err-border'
                : ''
          }`}
        >
          <div
            className={`mt-0.5 h-5 w-5 rounded-full flex items-center justify-center text-white text-[11px] font-bold flex-none ${
              t.kind === 'success'
                ? 'bg-ok'
                : t.kind === 'error'
                  ? 'bg-err'
                  : 'bg-primary'
            }`}
          >
            {t.kind === 'success' ? '✓' : t.kind === 'error' ? '!' : 'i'}
          </div>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-semibold">{t.title}</div>
            {t.body && (
              <div className="text-xs text-muted mt-0.5 leading-relaxed">
                {t.body}
              </div>
            )}
          </div>
          <button
            className="text-muted hover:text-ink text-xs"
            onClick={() => dismissToast(t.id)}
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  );
}
