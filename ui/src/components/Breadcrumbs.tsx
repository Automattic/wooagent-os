import { Link as RouterLink } from 'react-router-dom';
import { Link, Stack, Text } from '@wordpress/ui';

export interface BreadcrumbItem {
  label: string;
  to?: string;
}

interface Props {
  items: BreadcrumbItem[];
}

// Mirrors @wordpress/admin-ui's Breadcrumbs but uses react-router-dom Link
// (the canonical in-app link per DESIGN.md) instead of @wordpress/route.
// The last item with no `to` renders as the page h1 (heading-lg).
export default function Breadcrumbs({ items }: Props) {
  if (items.length === 0) return null;

  const preceding = items.slice(0, -1);
  const last = items[items.length - 1];

  return (
    <nav aria-label="Breadcrumbs">
      <Stack
        render={<ul />}
        direction="row"
        align="center"
        className="wa-breadcrumbs"
      >
        {preceding.map((item, index) => (
          <li key={index}>
            <Text
              variant="body-lg"
              render={
                <Link
                  tone="neutral"
                  render={<RouterLink to={item.to ?? '#'} />}
                />
              }
            >
              {item.label}
            </Text>
            <Text
              variant="body-lg"
              aria-hidden="true"
              className="wa-breadcrumbs__separator"
            >
              /
            </Text>
          </li>
        ))}
        <li>
          {last.to ? (
            <Text
              variant="body-lg"
              render={
                <Link
                  tone="neutral"
                  render={<RouterLink to={last.to} />}
                />
              }
            >
              {last.label}
            </Text>
          ) : (
            <Text
              variant="heading-lg"
              render={<h1 />}
              className="wa-breadcrumbs__current"
            >
              {last.label}
            </Text>
          )}
        </li>
      </Stack>
    </nav>
  );
}
