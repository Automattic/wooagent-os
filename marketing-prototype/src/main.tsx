import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import './styles/globals.css';

// `import.meta.env.BASE_URL` is set by Vite from the build-time `base` config.
// In dev it's `/`; on GHES Pages it's `/pages/Automattic/wooagent-os/`. React
// Router needs the trailing slash stripped when used as a basename.
const basename = import.meta.env.BASE_URL.replace(/\/$/, '') || '/';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter basename={basename}>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
