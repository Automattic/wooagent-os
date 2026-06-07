// Copy for post-action confirmation snackbars (write / revert / dismiss).
//
// Pure string templating: callers pass already-formatted values (prices,
// agent labels) so this module stays free of app types and is correct by
// inspection. Strings are approved against the WooAgent Testing Figma
// (write + dismiss rows); the revert rows and single-proposal marketing /
// sales-support rows are filled to match its voice.

export type ProposalKind =
  | 'product_price_change'
  | 'product_description_rewrite'
  | 'customer_reply_draft';

/** A formatted before→after price pair, e.g. { from: '$48.00', to: '$56.00' }. */
export interface PriceDiff {
  from: string;
  to: string;
}

/** Shown after a write lands on the store. */
export function writeConfirmText(kind: ProposalKind, diff?: PriceDiff): string {
  switch (kind) {
    case 'product_price_change':
      return diff ? `Price updated: ${diff.from} → ${diff.to}` : 'Price updated.';
    case 'product_description_rewrite':
      return 'Description updated.';
    case 'customer_reply_draft':
      return 'Reply posted.';
  }
}

/** customer_reply_draft has no undo path, so its write snackbar omits the
 *  inline Undo action. */
export function writeAllowsUndo(kind: ProposalKind): boolean {
  return kind !== 'customer_reply_draft';
}

/** Shown after an Undo reverts a write on the store. */
export function revertConfirmText(kind: ProposalKind, diff?: PriceDiff): string {
  switch (kind) {
    case 'product_price_change':
      return diff
        ? `Price change reverted: ${diff.from} → ${diff.to}`
        : 'Price change reverted.';
    case 'product_description_rewrite':
      return 'Rewrite reverted.';
    case 'customer_reply_draft':
      return 'Change reverted.';
  }
}

/** Shown after a dismiss. Marketing proposals carry multiple variants, so
 *  >1 reads "Variants archived"; everything else reads "Dismissed". Both
 *  note the agent was notified — dismiss feeds the prompt-tuning loop. */
export function dismissConfirmText(agentLabel: string, variantCount: number): string {
  const lead = variantCount > 1 ? 'Variants archived' : 'Dismissed';
  return `${lead} · ${agentLabel} notified`;
}

/** Shown after a batch approve-all. */
export function batchApproveText(kind: ProposalKind | undefined): string {
  return kind === 'product_price_change'
    ? 'Bulk price update applied.'
    : 'Bulk rewrite updated.';
}

/** Shown after a batch dismiss-all (children are archived together). */
export function batchDismissText(agentLabel: string): string {
  return `Variants archived · ${agentLabel} notified`;
}
