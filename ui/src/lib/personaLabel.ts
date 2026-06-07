// Canonical persona-slug → human label map. Shared by IssueDetail,
// BatchReview, and the snackbar copy so an agent is named the same way
// everywhere ("Pricing agent", not "pricing agent").
export function personaLabel(persona: string | undefined): string {
  switch (persona) {
    case 'marketing':
      return 'Marketing agent';
    case 'pricing':
      return 'Pricing agent';
    case 'sales-support':
      return 'Sales support agent';
    case 'inventory':
      return 'Inventory agent';
    case 'accounting':
      return 'Accounting agent';
    case 'reporting':
      return 'Reporting agent';
    case 'chief-of-staff':
      return 'Chief of staff';
    default:
      return `${persona ?? 'unassigned'} agent`;
  }
}
