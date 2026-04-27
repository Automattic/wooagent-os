// Thin client over the WooCommerce REST API. All requests go through the Vite
// dev proxy at `/api/woo/*` so the browser doesn't hit CORS — the proxy
// rewrites to `<store>/wp-json/wc/v3/*`. Auth is consumer-key/consumer-secret
// passed as query params, which is the standard for HTTPS Woo sites.

import type { StoreSettings } from './settings';

export interface WooProduct {
  id: number;
  name: string;
  sku: string;
  description: string;
  short_description: string;
  permalink: string;
  status: string;
}

export class WooApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

function authQuery(s: StoreSettings): string {
  if (!s.consumerKey || !s.consumerSecret) {
    throw new WooApiError(
      0,
      'Missing WooCommerce credentials. Open Settings and add a consumer key + secret.',
    );
  }
  const u = new URLSearchParams({
    consumer_key: s.consumerKey,
    consumer_secret: s.consumerSecret,
  });
  return u.toString();
}

async function woo<T>(
  s: StoreSettings,
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const url = `/api/woo${path}${path.includes('?') ? '&' : '?'}${authQuery(s)}`;
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(init.headers as Record<string, string> | undefined),
  };
  if (init.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  const res = await fetch(url, { ...init, headers });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body?.message) msg = body.message;
    } catch {
      /* not JSON */
    }
    throw new WooApiError(res.status, msg);
  }
  return (await res.json()) as T;
}

export async function listProducts(s: StoreSettings): Promise<WooProduct[]> {
  return woo<WooProduct[]>(s, '/products?per_page=20&status=publish');
}

export async function getProduct(
  s: StoreSettings,
  id: number,
): Promise<WooProduct> {
  return woo<WooProduct>(s, `/products/${id}`);
}

export async function updateProductDescription(
  s: StoreSettings,
  id: number,
  description: string,
): Promise<WooProduct> {
  return woo<WooProduct>(s, `/products/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ description }),
  });
}
