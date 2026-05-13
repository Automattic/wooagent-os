import { useState } from 'react';
import { Card, Notice, Stack, Text } from '@wordpress/ui';
import {
  Button,
  ExternalLink,
  SelectControl,
  TextControl,
} from '@wordpress/components';
import { Icon, check } from '@wordpress/icons';
import {
  api,
  type Connection,
  type ModelProvider,
  type ModelProviderKind,
  type ModelTestResult,
} from '../api/client';

interface Props {
  connection: Connection;
  onSaved(provider: ModelProvider): void;
  onBack(): void;
}

interface ProviderOption {
  kind: ModelProviderKind;
  title: string;
  blurb: string;
  defaultModel: string;
  // Models we list in the dropdown when the daemon doesn't return a list.
  knownModels?: string[];
  needsApiKey: boolean;
  needsEndpoint: boolean;
  // Tag rendered under the title — e.g. "Free · Local" for Ollama.
  tag?: { label: string; tone: 'success' | 'info' };
  // Provider console URL surfaced as an inline link below the API key
  // field. Lets operators without an account get one without leaving the
  // flow to search for it.
  apiKeyHelpUrl?: string;
  // Optional sub-line under the API-key field naming the supported
  // procurement tiers. For Anthropic this surfaces that Team / Enterprise
  // console keys work the same as personal — useful for org admins who'd
  // otherwise bounce off the personal-shaped placeholder.
  apiKeyTiers?: string;
}

const MUTED = { color: 'var(--wpds-color-fg-content-neutral-weak)' } as const;

const PROVIDERS: ProviderOption[] = [
  {
    kind: 'anthropic',
    title: 'Anthropic',
    blurb: 'Claude Opus, Sonnet, Haiku',
    defaultModel: 'claude-sonnet-4-6',
    knownModels: [
      'claude-opus-4-7',
      'claude-sonnet-4-6',
      'claude-haiku-4-5-20251001',
    ],
    needsApiKey: true,
    needsEndpoint: false,
    apiKeyHelpUrl: 'https://console.anthropic.com/settings/keys',
    apiKeyTiers: 'Personal, Team, and Enterprise console keys all work.',
  },
  {
    kind: 'openai',
    title: 'OpenAI',
    blurb: 'GPT-5 family',
    defaultModel: 'gpt-5',
    knownModels: ['gpt-5', 'gpt-5-mini', 'gpt-5-nano'],
    needsApiKey: true,
    needsEndpoint: false,
    apiKeyHelpUrl: 'https://platform.openai.com/api-keys',
  },
  {
    kind: 'ollama',
    title: 'Ollama (local)',
    blurb: 'Runs on your machine — no API key',
    defaultModel: '',
    needsApiKey: false,
    needsEndpoint: true,
    tag: { label: 'Free · Local', tone: 'success' },
  },
];

// Best-effort OS detection for the Ollama install hint. Falls through to
// "other" → we just point at ollama.com in that case.
function detectPlatform(): 'mac' | 'linux' | 'windows' | 'other' {
  if (typeof navigator === 'undefined') return 'other';
  const ua = navigator.userAgent;
  if (/Mac|iPhone|iPad/i.test(ua)) return 'mac';
  if (/Win/i.test(ua)) return 'windows';
  if (/Linux|X11/i.test(ua)) return 'linux';
  return 'other';
}

function ollamaInstallHint(): { command: string; note?: string } {
  switch (detectPlatform()) {
    case 'mac':
      return { command: 'brew install ollama' };
    case 'linux':
      return { command: 'curl -fsSL https://ollama.com/install.sh | sh' };
    case 'windows':
      return {
        command: '',
        note: 'Download the Windows installer from ollama.com.',
      };
    default:
      return { command: '', note: 'See ollama.com for install instructions.' };
  }
}

export default function Step4Model({ connection, onSaved, onBack }: Props) {
  const [picked, setPicked] = useState<ProviderOption | null>(null);
  const [apiKey, setApiKey] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const [model, setModel] = useState('');
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testResult, setTestResult] = useState<ModelTestResult | null>(null);
  const [discoveredModels, setDiscoveredModels] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const choose = (p: ProviderOption) => {
    setPicked(p);
    setApiKey('');
    setEndpoint(p.kind === 'ollama' ? 'http://localhost:11434' : '');
    setModel(p.defaultModel);
    setTestResult(null);
    setDiscoveredModels([]);
    setError(null);
  };

  const test = async () => {
    if (!picked) return;
    setError(null);
    setTesting(true);
    setTestResult(null);
    try {
      const res = await api.modelProviders.test(connection, {
        kind: picked.kind,
        api_key: picked.needsApiKey ? apiKey : undefined,
        endpoint: picked.needsEndpoint ? endpoint : undefined,
        default_model: model || undefined,
      });
      setTestResult(res);
      if (res.models && res.models.length > 0) {
        setDiscoveredModels(res.models);
        if (!model) setModel(res.models[0]);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setTesting(false);
    }
  };

  const save = async () => {
    if (!picked) return;
    setSaving(true);
    setError(null);
    try {
      const created = await api.modelProviders.create(connection, {
        kind: picked.kind,
        api_key: picked.needsApiKey ? apiKey : undefined,
        endpoint: picked.needsEndpoint ? endpoint : undefined,
        default_model: model,
      });
      onSaved(created);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const modelOptions =
    discoveredModels.length > 0
      ? discoveredModels
      : (picked?.knownModels ?? []);

  const canTest =
    !!picked &&
    (!picked.needsApiKey || apiKey.trim().length > 0) &&
    (!picked.needsEndpoint || endpoint.trim().length > 0);
  const canSave =
    canTest && testResult?.ok === true && model.trim().length > 0;

  return (
    <Card.Root>
      <Card.Content>
        <Stack direction="column" gap="xl">
          <Stack direction="column" gap="sm">
            <Text variant="heading-md">Configure a model</Text>
            <Text
              variant="body-sm"
              style={{ ...MUTED, textWrap: 'pretty' }}
            >
              Pick an LLM provider your agents will use — additional models
              can be added later. The credential is stored in your OS
              keychain so only WooAgent OS ever reads it.
            </Text>
          </Stack>

          <div className="wa-onboarding-providers">
            {PROVIDERS.map((p) => (
              // CUSTOM: LLM provider tile — selectable card with multi-line content. (a) WPDS has no selectable-tile / radio-card component. (b) <button> with .wa-onboarding-provider chrome + aria-pressed. (c) Follow-up: revisit if the onboarding-picker pattern repeats elsewhere.
              <button
                type="button"
                key={p.kind}
                onClick={() => choose(p)}
                className={`wa-onboarding-provider${
                  picked?.kind === p.kind
                    ? ' wa-onboarding-provider--selected'
                    : ''
                }`}
                aria-pressed={picked?.kind === p.kind}
              >
                <Stack direction="column" gap="xs" align="flex-start">
                  <Stack direction="row" gap="xs" align="center" wrap="wrap">
                    <Text
                      variant="body-md"
                      style={{
                        fontWeight:
                          'var(--wpds-typography-font-weight-medium)',
                      }}
                    >
                      {p.title}
                    </Text>
                    {p.tag && (
                      <span
                        className={`wa-onboarding-provider__tag wa-onboarding-provider__tag--${p.tag.tone}`}
                      >
                        {p.tag.label}
                      </span>
                    )}
                  </Stack>
                  <Text variant="body-sm" style={MUTED}>
                    {p.blurb}
                  </Text>
                </Stack>
                {picked?.kind === p.kind && (
                  <span
                    className="wa-onboarding-provider__check"
                    aria-hidden="true"
                  >
                    <Icon icon={check} size={14} />
                  </span>
                )}
              </button>
            ))}
          </div>

          {picked && (
            <Stack direction="column" gap="lg">
              {picked.needsEndpoint && (
                <Stack direction="column" gap="xs">
                  <TextControl
                    label="Endpoint"
                    value={endpoint}
                    onChange={(v: string | undefined) => {
                      setEndpoint(v ?? '');
                      setTestResult(null);
                    }}
                    placeholder="http://localhost:11434"
                    __next40pxDefaultSize
                    __nextHasNoMarginBottom
                  />
                  {picked.kind === 'ollama' &&
                    (() => {
                      const hint = ollamaInstallHint();
                      return (
                        <Text variant="body-sm" style={MUTED}>
                          Don't have Ollama?{' '}
                          {hint.command ? (
                            <>
                              Install with <code>{hint.command}</code> or see{' '}
                            </>
                          ) : (
                            <>{hint.note} </>
                          )}
                          <ExternalLink href="https://ollama.com/download">
                            ollama.com
                          </ExternalLink>
                          .
                        </Text>
                      );
                    })()}
                </Stack>
              )}
              {picked.needsApiKey && (
                <Stack direction="column" gap="xs">
                  <TextControl
                    label="API key"
                    value={apiKey}
                    onChange={(v: string | undefined) => {
                      setApiKey(v ?? '');
                      setTestResult(null);
                    }}
                    placeholder={
                      picked.kind === 'anthropic' ? 'sk-ant-…' : 'sk-…'
                    }
                    type="password"
                    help="Stored in your OS keychain. Never sent anywhere except the provider."
                    __next40pxDefaultSize
                    __nextHasNoMarginBottom
                  />
                  {picked.apiKeyTiers && (
                    <Text variant="body-sm" style={MUTED}>
                      {picked.apiKeyTiers}
                    </Text>
                  )}
                  {picked.apiKeyHelpUrl && (
                    <Text variant="body-sm" style={MUTED}>
                      Don't have one?{' '}
                      <ExternalLink href={picked.apiKeyHelpUrl}>
                        Get an API key from {picked.title}
                      </ExternalLink>
                      .
                    </Text>
                  )}
                </Stack>
              )}

              {modelOptions.length > 0 ? (
                <SelectControl
                  label="Default model"
                  value={model}
                  onChange={(v: string) => setModel(v)}
                  options={modelOptions.map((m) => ({ label: m, value: m }))}
                  __next40pxDefaultSize
                  __nextHasNoMarginBottom
                />
              ) : (
                <TextControl
                  label="Default model"
                  value={model}
                  onChange={(v: string | undefined) => setModel(v ?? '')}
                  placeholder={
                    picked.kind === 'ollama'
                      ? 'Click Test connection to detect models'
                      : 'model id'
                  }
                  help={
                    picked.kind === 'ollama' && testResult?.ok === false
                      ? 'Empty list — try `ollama pull llama3.2` and retry.'
                      : undefined
                  }
                  __next40pxDefaultSize
                  __nextHasNoMarginBottom
                />
              )}

              {testResult?.ok && (
                <Notice.Root intent="success">
                  <Notice.Description>
                    {testResult.message ?? 'Connection works.'}
                  </Notice.Description>
                </Notice.Root>
              )}
              {testResult && !testResult.ok && (
                <Notice.Root intent="error">
                  <Notice.Description>
                    {testResult.message ?? 'Test failed.'}
                  </Notice.Description>
                </Notice.Root>
              )}

              {error && (
                <Notice.Root intent="error">
                  <Notice.Description>{error}</Notice.Description>
                </Notice.Root>
              )}
            </Stack>
          )}

          <Stack direction="row" justify="space-between" align="center">
            <Button
              variant="tertiary"
              __next40pxDefaultSize
              onClick={onBack}
            >
              Back
            </Button>
            <Stack direction="row" gap="sm" align="center">
              <Button
                variant="secondary"
                __next40pxDefaultSize
                disabled={!canTest || testing}
                onClick={test}
              >
                {testing ? 'Testing…' : 'Test connection'}
              </Button>
              <Button
                variant="primary"
                __next40pxDefaultSize
                disabled={!canSave || saving}
                onClick={save}
              >
                {saving ? 'Saving…' : 'Save and continue'}
              </Button>
            </Stack>
          </Stack>
        </Stack>
      </Card.Content>
    </Card.Root>
  );
}
