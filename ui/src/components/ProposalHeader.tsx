// Shared title-row template for proposal detail/approval flows. Same persona
// eyebrow + thumbnail + heading + description rhythm across Marketing,
// Pricing, Sales Support, and any future agent's approval flow. Pair with the
// `wa-detail-title-row` class defined in app.css for the row layout.

import { Stack, Text } from '@wordpress/ui';
import { PersonaAvatar, personaKeyFrom } from './PersonaAvatar';
import ProductThumbnail from './ProductThumbnail';
import { relativeTime } from '../lib/boardItems';

interface Props {
  /** Raw persona slug from the issue (e.g. 'marketing', 'pricing', 'sales-support').
   *  Drives both the eyebrow avatar and the ProductThumbnail's placeholder
   *  fallback when no product image is bound. */
  persona: string;
  /** Display name of the persona ('Marketing', 'Pricing', 'Sales Support'). */
  personaLabel: string;
  /** Verb phrase shown after the persona name in the eyebrow
   *  ('proposes content', 'proposes a price change', 'drafted a customer reply'). */
  verb: string;
  /** ISO timestamp rendered as a relative time. */
  timestamp: string;
  /** Right-most segment of the eyebrow — model name + any tool annotations
   *  ('Claude Sonnet 4.6', 'Claude Haiku 4.5 · web_search'). */
  modelLine: string;
  /** heading-2xl title. */
  title: string;
  /** Body description rendered below the title. */
  description: string;
  /** Product image URL if the proposal target carries one. */
  imageUrl?: string;
  /** Alt text for the product image. */
  imageAlt?: string;
}

export default function ProposalHeader({
  persona,
  personaLabel,
  verb,
  timestamp,
  modelLine,
  title,
  description,
  imageUrl,
  imageAlt,
}: Props) {
  const personaKey = personaKeyFrom(persona);

  return (
    <>
      <Stack
        direction="row"
        gap="sm"
        align="center"
        style={{ marginBottom: 'var(--wpds-dimension-gap-xl)' }}
      >
        <PersonaAvatar persona={personaKey} size="md" />
        <Text
          variant="body-sm"
          style={{ color: 'var(--wpds-color-fg-content-neutral-weak)' }}
        >
          <strong style={{ color: 'var(--wpds-color-fg-content-neutral)' }}>
            {personaLabel}
          </strong>{' '}
          {verb} · {relativeTime(timestamp)} ·{' '}
          <span className="wa-mono">{modelLine}</span>
        </Text>
      </Stack>

      <div className="wa-detail-title-row">
        <ProductThumbnail
          src={imageUrl}
          alt={imageAlt}
          persona={persona}
          size="lg"
        />
        <div className="wa-detail-title-text">
          <Text
            variant="heading-2xl"
            render={
              <h2 style={{ margin: 0, marginBottom: 'var(--wpds-dimension-gap-sm)' }} />
            }
          >
            {title}
          </Text>
          <Text
            variant="body-md"
            style={{
              color: 'var(--wpds-color-fg-content-neutral-weak)',
              maxWidth: 760,
            }}
          >
            {description}
          </Text>
        </div>
      </div>
    </>
  );
}
