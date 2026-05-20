import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Page } from '@wordpress/admin-ui';
import { Badge, Card, Notice, Stack, Text } from '@wordpress/ui';
import {
  Button,
  SelectControl,
  Spinner,
  TextControl,
} from '@wordpress/components';

import { ApiError, api, type Connection, type Persona } from '../api/client';
import Breadcrumbs from '../components/Breadcrumbs';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';

interface Props {
  connection: Connection;
  /** Fired after a successful add so App can refetch sidebar counts +
   *  board. */
  onChanged?: () => void;
}

// Friendly cadence options. Values are seconds; the daemon stores
// cadence_seconds and the scheduler ticks on that interval.
const CADENCE_OPTIONS = [
  { label: 'Every hour', value: '3600' },
  { label: 'Every 6 hours', value: '21600' },
  { label: 'Every 12 hours', value: '43200' },
  { label: 'Daily', value: '86400' },
  { label: 'Weekly', value: '604800' },
];

// Model options mirror the inline ModelCell list on the Agents roster so
// the two surfaces stay in sync. Keep these aligned by hand for now;
// daemon-side model discovery lands in V2.
const MODEL_OPTIONS = [
  { label: 'Claude Sonnet 4.6', value: 'anthropic/claude-sonnet-4-6' },
  { label: 'Claude Opus 4.7', value: 'anthropic/claude-opus-4-7' },
  { label: 'Claude Haiku 4.5', value: 'anthropic/claude-haiku-4-5' },
  { label: 'Gemini 2.5 Pro', value: 'google/gemini-2.5-pro' },
  { label: 'GPT-5', value: 'openai/gpt-5' },
];

const DEFAULT_MODEL = 'anthropic/claude-sonnet-4-6';
const DEFAULT_CADENCE = '21600';

// Sentence-case display names matching Agents.tsx. Kept local here too so
// the picker doesn't import a screen module — the picker only shows
// dormant addable personas, a small set we can name explicitly.
function displayName(slug: string, fallback: string): string {
  if (slug === 'reporting') return 'Reporting';
  if (slug === 'inventory') return 'Inventory manager';
  if (slug === 'accounting') return 'Accounting';
  if (slug === 'chief') return 'Chief of staff';
  return fallback || slug;
}

// Editorial copy for each persona, used as the read-only "What this agent
// does" description on the form.
const PERSONA_DESCRIPTION: Record<string, string> = {
  reporting: 'Summarize store performance in weekly or monthly digests.',
  inventory:
    'Watches stock levels, flags low-stock and overstock situations, and drafts reorder proposals against your supplier list.',
  accounting:
    'Reconciles WooPayments and Stripe payouts against your bank, flags tax-relevant changes, and drafts month-end summaries.',
  chief:
    "Triages incoming work across your fleet, routes it to the right specialist, and keeps you briefed on what's drafted, approved, and waiting.",
};

// Personas the daemon doesn't register yet but that operators should
// still see in the picker so they can preview the configure form. The
// roster also renders these as "Coming soon" rows; this list lets the
// Add Agent page mirror that surface without faking a daemon row.
const COMING_SOON_SLUGS = ['inventory', 'accounting', 'chief', 'reporting'];

interface Candidate {
  slug: string;
  /** Display name shown in the picker + form. */
  name: string;
  /** Editorial description shown under the avatar. */
  description: string;
  /** 'available' lets the operator opt in; 'coming-soon' fills the form
   *  for preview but disables the Add button. */
  status: 'available' | 'coming-soon';
}

export default function AddAgent({ connection, onChanged }: Props) {
  const navigate = useNavigate();
  const [agents, setAgents] = useState<Persona[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [selectedSlug, setSelectedSlug] = useState<string>('');
  const [name, setName] = useState<string>('');
  const [model, setModel] = useState<string>(DEFAULT_MODEL);
  const [cadence, setCadence] = useState<string>(DEFAULT_CADENCE);

  useEffect(() => {
    const signal = { cancelled: false };
    (async () => {
      try {
        const res = await api.agents(connection);
        if (signal.cancelled) return;
        setAgents(res.agents);
      } catch (e) {
        if (!signal.cancelled) {
          setLoadError(e instanceof Error ? e.message : String(e));
        }
      }
    })();
    return () => {
      signal.cancelled = true;
    };
  }, [connection]);

  // Candidates the operator can see in the picker: truly addable
  // personas first (daemon-registered, dormant), then coming-soon previews
  // for the personas we plan to ship. Both fill the form on selection so
  // the operator can see what configuration will look like; only
  // 'available' candidates can actually be saved.
  const candidates = useMemo<Candidate[]>(() => {
    if (!agents) return [];
    // Personas actually working on this fleet: registered AND (enabled OR
    // not addable). A persona whose agents row says enabled=1 but whose
    // Go type is no longer registered (e.g. Reporting today — historical
    // row survives an un-Register) is *not* operable, so it should still
    // surface in the picker so the operator can see it's coming back.
    const operableSlugs = new Set(
      agents
        .filter((a) => a.implemented && (a.enabled || !a.addable))
        .map((a) => a.persona),
    );
    const out: Candidate[] = [];

    for (const a of agents) {
      if (operableSlugs.has(a.persona)) continue;
      if (!a.implemented || !a.addable) continue;
      out.push({
        slug: a.persona,
        name: displayName(a.persona, a.name),
        description:
          PERSONA_DESCRIPTION[a.persona] ??
          'Drafts proposals on a cadence you set, lands them in your review queue.',
        status: 'available',
      });
    }

    for (const slug of COMING_SOON_SLUGS) {
      if (operableSlugs.has(slug)) continue;
      if (out.some((c) => c.slug === slug)) continue;
      out.push({
        slug,
        name: displayName(slug, slug),
        description:
          PERSONA_DESCRIPTION[slug] ??
          'Drafts proposals on a cadence you set, lands them in your review queue.',
        status: 'coming-soon',
      });
    }

    return out;
  }, [agents]);

  // Default the picker to the first available candidate, or the first
  // coming-soon entry if everything is in preview. Prime the name field
  // from the candidate's display name so the operator can keep or rename.
  useEffect(() => {
    if (!candidates.length) return;
    if (selectedSlug && candidates.some((c) => c.slug === selectedSlug)) return;
    const firstAvailable = candidates.find((c) => c.status === 'available');
    const first = firstAvailable ?? candidates[0];
    setSelectedSlug(first.slug);
    setName(first.name);
  }, [candidates, selectedSlug]);

  const selected = useMemo(
    () => candidates.find((c) => c.slug === selectedSlug) ?? null,
    [candidates, selectedSlug],
  );

  function handlePersonaChange(slug: string) {
    setSelectedSlug(slug);
    const match = candidates.find((c) => c.slug === slug);
    if (match) setName(match.name);
  }

  async function handleSave() {
    if (!selected || selected.status !== 'available') return;
    setSaving(true);
    setSaveError(null);
    try {
      await api.patchAgent(connection, selected.slug, {
        enabled: true,
        name: name.trim() || selected.name,
        model_preference: model,
        cadence_seconds: Number(cadence),
      });
      onChanged?.();
      navigate('/agents');
    } catch (e) {
      const msg =
        e instanceof ApiError
          ? `${e.code}: ${e.message}`
          : e instanceof Error
            ? e.message
            : String(e);
      setSaveError(msg);
      setSaving(false);
    }
  }

  const canSave = !!selected && selected.status === 'available' && !saving;
  const showComingSoon = selected?.status === 'coming-soon';

  return (
    <Page
      breadcrumbs={
        <Breadcrumbs
          items={[
            { label: 'Agents', to: '/agents' },
            { label: 'Add agent' },
          ]}
        />
      }
      badges={showComingSoon ? <Badge intent="none">Coming soon</Badge> : null}
      hasPadding
      actions={
        <Stack direction="row" gap="sm" align="center">
          <Button
            variant="tertiary"
            __next40pxDefaultSize
            onClick={() => navigate('/agents')}
            disabled={saving}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            __next40pxDefaultSize
            isBusy={saving}
            disabled={!canSave}
            onClick={() => void handleSave()}
          >
            Save
          </Button>
        </Stack>
      }
    >
      {/* Form-width subpage: matches the 800px-ish form width in the
          Figma. Slightly tighter than .wa-subpage-content's 1100px which
          is sized for queue + detail surfaces, not narrow editorial forms. */}
      <div
        className="wa-subpage-content"
        style={{ maxWidth: 'var(--wpds-dimension-surface-width-xl)' }}
      >
        {loadError ? (
          <Notice.Root intent="error">
            <Notice.Description>
              Couldn't load your agents right now. ({loadError})
            </Notice.Description>
          </Notice.Root>
        ) : agents === null ? (
          <Stack direction="row" gap="sm" align="center">
            <Spinner />
            <Text variant="body-sm">Getting your agents ready…</Text>
          </Stack>
        ) : candidates.length === 0 ? (
          <Stack direction="column" gap="md">
            <Text variant="body-md">
              Every available agent is already on your fleet. Future personas
              will show up here as we ship them.
            </Text>
            <div>
              <Button
                variant="secondary"
                __next40pxDefaultSize
                onClick={() => navigate('/agents')}
              >
                Back to fleet
              </Button>
            </div>
          </Stack>
        ) : (
          <Stack direction="column" gap="lg">
            {saveError && (
              <Notice.Root intent="error">
                <Notice.Description>
                  Couldn't add agent: {saveError}
                </Notice.Description>
                <Notice.CloseIcon
                  label="Dismiss notice"
                  onClick={() => setSaveError(null)}
                />
              </Notice.Root>
            )}

            <Card.Root>
              <Card.Header>
                <Text variant="heading-md" render={<h2 />}>
                  Identity
                </Text>
              </Card.Header>
              <Card.Content>
                <Stack direction="column" gap="md">
                  <SelectControl
                    __nextHasNoMarginBottom
                    label="Agent"
                    value={selectedSlug}
                    options={candidates.map((c) => ({
                      label:
                        c.status === 'coming-soon'
                          ? `${c.name} (coming soon)`
                          : c.name,
                      value: c.slug,
                    }))}
                    onChange={handlePersonaChange}
                    disabled={saving}
                  />

                  {selected && (
                    <Stack direction="row" gap="sm" align="center">
                      <PersonaAvatar
                        persona={personaKeyFrom(selected.slug)}
                        size="md"
                      />
                      <Text
                        variant="body-sm"
                        style={{ color: 'var(--wpds-color-fg-content-neutral)' }}
                      >
                        {selected.description}
                      </Text>
                    </Stack>
                  )}

                  <TextControl
                    __nextHasNoMarginBottom
                    __next40pxDefaultSize
                    label="Name"
                    help="How this agent appears in your roster and on proposals."
                    value={name}
                    onChange={setName}
                    disabled={saving}
                  />
                </Stack>
              </Card.Content>
            </Card.Root>

            <Card.Root>
              <Card.Header>
                <Text variant="heading-md" render={<h2 />}>
                  Runtime
                </Text>
              </Card.Header>
              <Card.Content>
                <Stack direction="column" gap="md">
                  <SelectControl
                    __nextHasNoMarginBottom
                    label="Model"
                    help="Which model this agent uses when it drafts."
                    value={model}
                    options={MODEL_OPTIONS}
                    onChange={setModel}
                    disabled={saving}
                  />
                  <SelectControl
                    __nextHasNoMarginBottom
                    label="Cadence"
                    help="How often this agent looks for work."
                    value={cadence}
                    options={CADENCE_OPTIONS}
                    onChange={setCadence}
                    disabled={saving}
                  />
                </Stack>
              </Card.Content>
            </Card.Root>
          </Stack>
        )}
      </div>
    </Page>
  );
}
