// Settings live in localStorage so the operator can configure once and reload
// without re-entering credentials. Consumer key/secret never leave the
// browser — they're sent only to `/api/woo/*`, which Vite proxies to the
// configured WooCommerce store.

const KEY = 'wooagent.marketing.settings';

export interface StoreSettings {
  storeLabel: string;
  consumerKey: string;
  consumerSecret: string;
  productId: number | null;
}

export const DEFAULT_SETTINGS: StoreSettings = {
  storeLabel: 'woo-demo-store-99cc5c.mystagingwebsite.com',
  consumerKey: '',
  consumerSecret: '',
  productId: null,
};

export function loadSettings(): StoreSettings {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return { ...DEFAULT_SETTINGS };
    const parsed = JSON.parse(raw) as Partial<StoreSettings>;
    return { ...DEFAULT_SETTINGS, ...parsed };
  } catch {
    return { ...DEFAULT_SETTINGS };
  }
}

export function saveSettings(s: StoreSettings): void {
  localStorage.setItem(KEY, JSON.stringify(s));
}
