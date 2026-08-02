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
