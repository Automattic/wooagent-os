import { useEffect } from 'react';

// Publishes the rendered `.wa-action-bar` height to the `--wa-action-bar-height`
// CSS variable on the document root, so fixed-position overlays (the
// confirmation snackbar) can sit just above the bar without hardcoding its
// height — the WPDS padding tokens the bar is built from are breakpoint-
// variable. A ResizeObserver keeps the value correct across breakpoint
// changes and bar state swaps (review → done). Safe to query the DOM because
// at most one action bar is mounted at a time.
//
// `deps` should change whenever the bar mounts/unmounts or swaps state so the
// effect re-binds to the current element.
export function useActionBarHeightVar(deps: unknown[]): void {
  useEffect(() => {
    const el = document.querySelector('.wa-action-bar') as HTMLElement | null;
    const root = document.documentElement;
    if (!el) {
      root.style.removeProperty('--wa-action-bar-height');
      return;
    }
    const apply = () =>
      root.style.setProperty('--wa-action-bar-height', `${el.offsetHeight}px`);
    apply();
    const observer = new ResizeObserver(apply);
    observer.observe(el);
    return () => {
      observer.disconnect();
      root.style.removeProperty('--wa-action-bar-height');
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}
