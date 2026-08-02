import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Page } from '@wordpress/admin-ui';
import { Badge, Card, Notice, Stack, Text } from '@wordpress/ui';
import {
  Button,
  SelectControl,
  Spinner,
  TextareaControl,
  TextControl,
} from '@wordpress/components';

import { ApiError, api, type Connection, type Persona } from '../api/client';
import Breadcrumbs from '../components/Breadcrumbs';
import { PersonaAvatar, personaKeyFrom } from '../components/PersonaAvatar';

interface Props {
  connection: Connection;
  /** Fired after a successful save so App can refetch sidebar counts +
   *  board. */
  onChanged?: () => void;
}

// Mirrors AddAgent.tsx's options. Keep them in sync by hand for now;
// daemon-side cadence/model discovery lands in V2.
const CADENCE_OPTIONS = [
  { label: 'Every hour', value: '3600' },
  { label: 'Every 6 hours', value: '21600' },
  { label: 'Every 12 hours', value: '43200' },
  { label: 'Daily', value: '86400' },
  { label: 'Weekly', value: '604800' },
];

const MODEL_OPTIONS = [
  { label: 'Claude Sonnet 4.6', value: 'anthropic/claude-sonnet-4-6' },
  { label: 'Claude Opus 4.7', value: 'anthropic/claude-opus-4-7' },
  { label: 'Claude Haiku 4.5', value: 'anthropic/claude-haiku-4-5' },
  { label: 'Gemini 2.5 Pro', value: 'google/gemini-2.5-pro' },
  { label: 'GPT-5', value: 'openai/gpt-5' },
];

const DEFAULT_MODEL = 'anthropic/claude-sonnet-4-6';
const DEFAULT_CADENCE = '21600';

// Sentence-case display names matching Agents.tsx / AddAgent.tsx.
function displayName(slug: string, fallback: string): string {
  if (slug === 'marketing') return 'Marketing & SEO';
  if (slug === 'pricing') return 'Pricing';
  if (slug === 'sales-support') return 'Sales support';
  if (slug === 'reporting') return 'Reporting';
  if (slug === 'inventory') return 'Inventory manager';
  if (slug === 'accounting') return 'Accounting';
  if (slug === 'chief') return 'Chief of staff';
  if (!fallback) return slug;
  return fallback.charAt(0).toUpperCase() + fallback.slice(1).toLowerCase();
}

// UI-side system prompts shown on the Edit Agent page. These are stubs —
// the canonical prompts live in daemon Go code (e.g. sales-support's
// `const systemPrompt`) and skill YAML templates. There's no daemon
// endpoint yet to fetch them, so the operator sees illustrative copy
// matching what EditAgentModal previously showed. Promote to a real
// GET /v1/agents/{slug}/prompt when the daemon exposes it.
const SYSTEM_PROMPT: Record<string, string> = {
  marketing:
    "You are the Marketing & SEO agent for mystore.com. Your primary goal is to grow organic traffic and improve conversion rates. Monitor keyword rankings, suggest meta description updates, and generate product copy that matches the store's warm, approachable brand voice. Always flag changes before writing to WooCommerce.",
  pricing:
    'You are the Pricing agent. Watch margins, sales velocity, and competitor signals to propose price moves. Always require human approval before changing live prices.',
  'sales-support':
    'You are the Sales Support agent. Draft customer replies, handle refund triage, and escalate edge cases. Never reply directly without human approval.',
  inventory:
    'You are the Inventory agent. Surface low-stock and overstock issues, propose reorder quantities, and draft purchase orders against approved suppliers.',
  accounting:
    'You are the Accounting agent. Reconcile WooPayments and Stripe payouts against the bank, flag tax-relevant changes, and draft month-end summaries.',
  reporting:
    'You are the Reporting agent. Generate weekly and monthly digests covering revenue, conversion, and operational anomalies. Investigate ad-hoc questions on request.',
  chief:
    'You are the Chief of Staff. Triage incoming work, route it to the right specialist agent, and keep the operator briefed on what the fleet is doing.',
};

// Editorial copy shown under the persona avatar on the form. Kept aligned
// with AddAgent.tsx's PERSONA_DESCRIPTION + Agents.tsx's PERSONA_META.mandate.
const PERSONA_DESCRIPTION: Record<string, string> = {
  marketing: 'Grows organic traffic and on-site conversion.',
  pricing: 'Protects margin and monitors competitor pricing.',
  'sales-support': 'Drafts replies to pre-sale and order inquiries.',
  reporting: 'Summarize store performance in weekly or monthly digests.',
  inventory:
    'Watches stock levels, flags low-stock and overstock situations, and drafts reorder proposals against your supplier list.',
  accounting:
    'Reconciles WooPayments and Stripe payouts against your bank, flags tax-relevant changes, and drafts month-end summaries.',
  chief:
    "Triages incoming work across your fleet, routes it to the right specialist, and keeps you briefed on what's drafted, approved, and waiting.",
};

export default function EditAgent({ connection, onChanged }: Props) {
  const navigate = useNavigate();
  const { slug = '' } = useParams<{ slug: string }>();

  const [persona, setPersona] = useState<Persona | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [name, setName] = useState<string>('');
  const [model, setModel] = useState<string>(DEFAULT_MODEL);
  const [cadence, setCadence] = useState<string>(DEFAULT_CADENCE);

  // Load the full agents list and pick out the row for this slug. We use
  // /v1/agents (not a per-slug fetch) because that endpoint is the only
  // one that returns the canonical Persona shape with implemented/addable
  // flags — useful for catching a stale URL pointing at an unregistered
  // persona.
  useEffect(() => {
    const signal = { cancelled: false };
    (async () => {
      try {
        const res = await api.agents(connection);
        if (signal.cancelled) return;
        const found = res.agents.find((a) => a.persona === slug) ?? null;
        setPersona(found);
        if (found) {
          setName(displayName(found.persona, found.name));
          setModel(found.model_preference || DEFAULT_MODEL);
          setCadence(String(found.cadence_seconds ?? DEFAULT_CADENCE));
        } else {
          setLoadError(`No agent named "${slug}" is registered.`);
        }
      } catch (e) {
        if (!signal.cancelled) {
          setLoadError(e instanceof Error ? e.message : String(e));
        }
      }
    })();
    return () => {
      signal.cancelled = true;
    };
  }, [connection, slug]);

  async function handleSave() {
    if (!persona) return;
    setSaving(true);
    setSaveError(null);
    try {
      await api.patchAgent(connection, persona.persona, {
        name: name.trim() || displayName(persona.persona, persona.name),
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

  const canSave = !!persona && !saving;
  const personaLabel = persona ? displayName(persona.persona, persona.name) : '';

  return (
    <Page
      breadcrumbs={
        <Breadcrumbs
          items={[
            { label: 'Agents', to: '/agents' },
            { label: personaLabel || 'Edit agent' },
          ]}
        />
      }
      badges={
        persona ? (
          <Badge intent={persona.enabled ? 'stable' : 'draft'}>
            {persona.enabled ? 'Active' : 'Inactive'}
          </Badge>
        ) : null
      }
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
      {/* Same form width as Add Agent — see AddAgent.tsx for rationale. */}
      <div
        className="wa-subpage-content"
        style={{ maxWidth: 'var(--wpds-dimension-surface-width-xl)' }}
      >
        {loadError ? (
          <Notice.Root intent="error">
            <Notice.Description>
              Couldn't load this agent: {loadError}
            </Notice.Description>
          </Notice.Root>
        ) : persona === null ? (
          <Stack direction="row" gap="sm" align="center">
            <Spinner />
            <Text variant="body-sm">Getting your agent ready…</Text>
          </Stack>
        ) : (
          <Stack direction="column" gap="lg">
            {saveError && (
              <Notice.Root intent="error">
                <Notice.Description>
                  Couldn't save: {saveError}
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
                  <Stack direction="row" gap="sm" align="center">
                    <PersonaAvatar
                      persona={personaKeyFrom(persona.persona)}
                      size="md"
                    />
                    <Text
                      variant="body-sm"
                      style={{ color: 'var(--wpds-color-foreground-content-neutral)' }}
                    >
                      {PERSONA_DESCRIPTION[persona.persona] ??
                        "Drafts proposals on a cadence you set, lands them in your review queue."}
                    </Text>
                  </Stack>

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

            <Card.Root>
              <Card.Header>
                <Text variant="heading-md" render={<h2 />}>
                  Prompt
                </Text>
              </Card.Header>
              <Card.Content>
                <TextareaControl
                  __nextHasNoMarginBottom
                  label="System prompt"
                  help="The instructions this agent reads before each run. Editing isn't wired up yet — coming soon."
                  value={SYSTEM_PROMPT[persona.persona] ?? ''}
                  onChange={() => {
                    /* read-only for now */
                  }}
                  readOnly
                  rows={8}
                />
              </Card.Content>
            </Card.Root>
          </Stack>
        )}
      </div>
    </Page>
  );
}
