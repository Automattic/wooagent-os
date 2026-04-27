import { Link } from 'react-router-dom';

interface Props {
  area: string;
  description: string;
  status?: 'soon' | 'demo';
  ctaTo?: string;
  ctaLabel?: string;
}

export default function Placeholder({
  area,
  description,
  status = 'soon',
  ctaTo = '/',
  ctaLabel = '← Back to Board',
}: Props) {
  return (
    <main className="max-w-[900px] mx-auto px-6 py-16">
      <div
        className="card p-10 text-center"
        style={{
          background:
            'radial-gradient(circle at 30% 20%, #FCE7F3 0%, transparent 60%), #FFFFFF',
        }}
      >
        <div
          className="eyebrow mb-3 inline-block"
          style={{
            color: status === 'soon' ? '#BE185D' : '#0066CC',
          }}
        >
          {status === 'soon' ? 'Out of scope · prototype' : 'Reference area'}
        </div>
        <h1 className="display text-3xl font-semibold mb-3">{area}</h1>
        <p className="text-sm text-muted max-w-xl mx-auto leading-relaxed">
          {description}
        </p>
        <div className="mt-6">
          <Link to={ctaTo} className="btn btn-primary text-xs no-underline">
            {ctaLabel}
          </Link>
        </div>
      </div>
    </main>
  );
}
