import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import '@wordpress/components/build-style/style.css';
import './styles/globals.css';

// `import.meta.env.BASE_URL` is set by Vite from the build-time `base` config.
// In dev it's `/`; on GHES Pages it's `/pages/Automattic/wooagent-os/`. React
// Router needs the trailing slash stripped when used as a basename.
const basename = import.meta.env.BASE_URL.replace(/\/$/, '') || '/';

// `@wordpress/theme/design-tokens.css` is imported via globals.css. That alone
// defines every `--wpds-*` token on :root. ThemeProvider exists in the package
// but is currently locked behind privateApis (experimental in 0.11.x), so we
// rely on the static stylesheet — sufficient for our needs (no per-subtree
// theming, no dark mode override).

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter basename={basename}>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
