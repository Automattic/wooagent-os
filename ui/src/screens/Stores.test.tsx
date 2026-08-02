import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import Stores from './Stores';
import { AskAgentProvider } from '../lib/askAgent';
import { api, type Connection, type McpMismatch, type Store } from '../api/client';

// The mismatch notice is the UI half of the fix for a silent-divergence bug:
// the daemon warns on stdout when WOOAGENT_MCP_URL names a different store
// than the paired one, and stdout is invisible to anyone running the app in a
// window. A conditional render is easy to drop in a refactor without anything
// failing, so it's worth pinning both branches.

const CONNECTION: Connection = { daemonUrl: 'http://daemon.test', token: 'tok' };

const PAIRED_STORE: Store = {
  id: 'store_1',
  url: 'https://real-store.example.com',
  status: 'paired',
  paired_at: '2026-08-02T15:40:04Z',
};

function renderStores() {
  return render(
    // Stores calls useAskAgentContext to register its page context, so it
    // needs the provider and a router to mount at all.
    <MemoryRouter>
      <AskAgentProvider>
        <Stores
          connection={CONNECTION}
          onAskAgent={() => {}}
          onStoreDisconnected={() => {}}
        />
      </AskAgentProvider>
    </MemoryRouter>,
  );
}

function stubStoresList(mismatch: McpMismatch | null) {
  return vi
    .spyOn(api.stores, 'list')
    .mockResolvedValue({ stores: [PAIRED_STORE], mcp_mismatch: mismatch });
}

describe('Stores — MCP store mismatch notice', () => {
  it('names both stores when the env var disagrees with the paired store', async () => {
    stubStoresList({
      env_host: 'stale-store.example.com',
      paired_host: 'real-store.example.com',
    });

    renderStores();

    // Both hosts have to appear: which one is being ignored, and which one is
    // actually in use. Naming only one leaves the operator guessing.
    await waitFor(() => {
      expect(screen.getByText('stale-store.example.com')).toBeInTheDocument();
    });
    expect(screen.getByText('real-store.example.com')).toBeInTheDocument();
    expect(screen.getByText('WOOAGENT_MCP_URL')).toBeInTheDocument();
  });

  it('stays quiet when the daemon reports no mismatch', async () => {
    stubStoresList(null);

    renderStores();

    await waitFor(() => {
      expect(api.stores.list).toHaveBeenCalled();
    });
    expect(screen.queryByText('WOOAGENT_MCP_URL')).not.toBeInTheDocument();
  });

  it('stays quiet when an older daemon omits the field entirely', async () => {
    // Forward compatibility runs both ways: the UI and daemon ship
    // independently, so a UI build can meet a daemon that predates
    // mcp_mismatch. Absent must read the same as "no mismatch", not throw.
    vi.spyOn(api.stores, 'list').mockResolvedValue({ stores: [PAIRED_STORE] });

    renderStores();

    await waitFor(() => {
      expect(api.stores.list).toHaveBeenCalled();
    });
    expect(screen.queryByText('WOOAGENT_MCP_URL')).not.toBeInTheDocument();
  });
});

describe('Stores — capability gaps', () => {
  it('names each missing ability and who needs it', async () => {
    vi.spyOn(api.stores, 'list').mockResolvedValue({
      stores: [
        {
          ...PAIRED_STORE,
          capability_gaps: [
            { ability: 'wooagent-products/update', used_by: 'Marketing', missing: true },
            {
              ability: 'wooagent-products/list',
              used_by: 'Pricing',
              unsupported_params: ['orderby', 'order'],
            },
          ],
        },
      ],
      mcp_mismatch: null,
    });

    renderStores();

    // The whole point is actionability: which ability, and which agent
    // stops working without it.
    await waitFor(() => {
      expect(
        screen.getByText(
          'wooagent-products/update is not available on this store (needed by Marketing)',
        ),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText(
        'wooagent-products/list does not accept orderby, order on this store (needed by Pricing)',
      ),
    ).toBeInTheDocument();
    // And the fix, since "missing ability" isn't self-explanatory.
    // getAllByText, not getByText: WPDS Notice mirrors its content into an
    // aria-live region for screen readers, so a substring match legitimately
    // hits both the visible copy and the announced one.
    expect(
      screen.getAllByText(/Update the WooAgent Companion Plugin/).length,
    ).toBeGreaterThan(0);
  });

  it('stays quiet for a store with no gaps', async () => {
    vi.spyOn(api.stores, 'list').mockResolvedValue({
      stores: [{ ...PAIRED_STORE, capability_gaps: [] }],
      mcp_mismatch: null,
    });

    renderStores();

    await waitFor(() => {
      expect(api.stores.list).toHaveBeenCalled();
    });
    expect(
      screen.queryAllByText(/Update the WooAgent Companion Plugin/),
    ).toHaveLength(0);
  });

  it('stays quiet when an older daemon omits capability_gaps', async () => {
    // Same independent-shipping concern as mcp_mismatch: a UI build can meet
    // a daemon that predates capability_gaps. Absent has to read as "no
    // gaps" rather than throwing on `.length` of undefined.
    vi.spyOn(api.stores, 'list').mockResolvedValue({ stores: [PAIRED_STORE] });

    renderStores();

    await waitFor(() => {
      expect(api.stores.list).toHaveBeenCalled();
    });
    expect(
      screen.queryAllByText(/Update the WooAgent Companion Plugin/),
    ).toHaveLength(0);
  });
});
