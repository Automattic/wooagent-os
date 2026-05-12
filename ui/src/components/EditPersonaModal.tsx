import { useState } from 'react';
import { Modal, Button } from '@wordpress/components';
import { Icon, chevronDown } from '@wordpress/icons';
import { Stack, Text } from '@wordpress/ui';
import { type Persona } from '../api/client';
import { PersonaAvatar, personaKeyFrom } from './PersonaAvatar';

interface Props {
  persona: Persona;
  mandate: string;
  systemPrompt: string;
  onClose: () => void;
}

const PROMPT_LIMIT = 2000;

const BEHAVIORS = [
  { id: 'draft-only', label: 'Draft only — never auto-apply', defaultActive: true },
  { id: 'require-approval', label: 'Require human approval', defaultActive: true },
  { id: 'daily-schedule', label: 'Daily schedule', defaultActive: false },
  { id: 'alert-errors', label: 'Alert on errors', defaultActive: true },
];

// Visual-only V1 edit modal. The daemon doesn't expose a PATCH /v1/agents
// yet, so Save/Cancel/Delete all just close. Local state lets the operator
// see the controls behave; nothing persists.
export default function EditPersonaModal({ persona, systemPrompt, onClose }: Props) {
  const personaKey = personaKeyFrom(persona.persona);
  // Sentence case for all persona display names (DESIGN.md). Acronyms (SEO)
  // remain uppercased.
  const fallback = persona.name || persona.persona;
  const displayName =
    persona.persona === 'marketing'
      ? 'Marketing & SEO'
      : persona.persona === 'inventory'
        ? 'Inventory manager'
        : persona.persona === 'sales-support'
          ? 'Sales support'
          : persona.persona === 'chief'
            ? 'Chief of staff'
            : fallback.charAt(0).toUpperCase() + fallback.slice(1).toLowerCase();

  const [prompt, setPrompt] = useState(systemPrompt);
  const [behaviors, setBehaviors] = useState<Record<string, boolean>>(
    () => Object.fromEntries(BEHAVIORS.map((b) => [b.id, b.defaultActive])),
  );

  return (
    <Modal
      title=""
      contentLabel={`Edit ${displayName}`}
      onRequestClose={onClose}
      __experimentalHideHeader
      size="medium"
      className="wa-edit-persona-modal"
    >
      <Stack direction="column" gap="md">
        <Stack
          direction="row"
          justify="space-between"
          align="center"
          style={{ marginBottom: 'var(--wpds-dimension-gap-sm)' }}
        >
          <Stack direction="row" gap="sm" align="center">
            <PersonaAvatar persona={personaKey} size="md" />
            <Stack direction="column" gap="xs">
              <Text
                variant="heading-sm"
                style={{
                  fontWeight: 'var(--wpds-typography-font-weight-medium)',
                }}
              >
                {displayName}
              </Text>
              <span
                style={{
                  fontSize: 'var(--wpds-typography-font-size-xs)',
                  color: 'var(--wpds-color-fg-content-neutral-weak)',
                }}
              >
                {persona.persona}
              </span>
            </Stack>
          </Stack>
        </Stack>

        <Stack direction="column" gap="xs">
          <span className="wa-eyebrow">Model</span>
          {/* CUSTOM: model-picker affordance — currently a stub, no menu wired. (a) WPDS SelectControl doesn't render a mono-value + chevron disclosure shape. (b) one-shot disclosure stub with .wa-roster__model chrome. (c) Follow-up: migrate to WPDS Dropdown when the picker is wired. */}
          <button type="button" className="wa-roster__model wa-edit-persona-modal__select">
            <span className="wa-mono">
              {persona.model_preference ?? 'claude-sonnet-4.6'}
            </span>
            <Icon icon={chevronDown} size={14} />
          </button>
        </Stack>

        <Stack direction="column" gap="xs">
          <Stack direction="row" justify="space-between" align="center">
            <span className="wa-eyebrow">System prompt</span>
            <span
              className="wa-mono"
              style={{
                fontSize: 'var(--wpds-typography-font-size-xs)',
                color: 'var(--wpds-color-fg-content-neutral-weak)',
              }}
            >
              {prompt.length} / {PROMPT_LIMIT}
            </span>
          </Stack>
          <textarea
            className="wa-edit-persona-modal__prompt"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value.slice(0, PROMPT_LIMIT))}
            rows={6}
          />
        </Stack>

        <Stack direction="column" gap="xs">
          <span className="wa-eyebrow">Behavior</span>
          <div className="wa-edit-persona-modal__chips">
            {BEHAVIORS.map((b) => {
              const active = behaviors[b.id];
              // CUSTOM: behavior chip toggle. (a) WPDS has no chip-group / multi-select toggle — FormToggle is a single boolean, Button has no selected/pressed state, no chip-group primitive. (b) pill with .wa-chip + .wa-chip--active state. (c) Follow-up: propose a ToggleGroupControl-style chip pattern in #design-systems.
              return (
                <button
                  key={b.id}
                  type="button"
                  className={`wa-chip${active ? ' wa-chip--active' : ''}`}
                  onClick={() =>
                    setBehaviors((prev) => ({ ...prev, [b.id]: !prev[b.id] }))
                  }
                >
                  {b.label}
                </button>
              );
            })}
          </div>
        </Stack>

        <Stack direction="row" justify="space-between" align="center" style={{ marginTop: 'var(--wpds-dimension-gap-md)' }}>
          <Button variant="tertiary" __next40pxDefaultSize isDestructive onClick={onClose}>
            Delete agent
          </Button>
          <Stack direction="row" gap="sm" align="center">
            <Button variant="secondary" __next40pxDefaultSize onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" __next40pxDefaultSize onClick={onClose}>
              Save changes
            </Button>
          </Stack>
        </Stack>
      </Stack>
    </Modal>
  );
}
