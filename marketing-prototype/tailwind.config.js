/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
        serif: ['"Source Serif 4"', 'Georgia', 'serif'],
      },
      colors: {
        ink: {
          DEFAULT: '#09090B',
          soft: '#27272A',
        },
        muted: {
          DEFAULT: '#71717A',
          2: '#A1A1AA',
        },
        border: {
          DEFAULT: '#ECECEE',
          strong: '#D4D4D8',
        },
        primary: {
          DEFAULT: '#0066CC',
          hover: '#0052A3',
        },
        mk: {
          DEFAULT: '#BE185D',
          bg: '#FCE7F3',
          ink: '#831843',
        },
        ok: {
          DEFAULT: '#16A34A',
          bg: '#F0FDF4',
          border: '#BBF7D0',
        },
        warn: {
          DEFAULT: '#CA8A04',
          bg: '#FEFCE8',
          border: '#FDE68A',
          strong: '#A16207',
        },
        err: {
          DEFAULT: '#DC2626',
          bg: '#FEF2F2',
          border: '#FECACA',
        },
        info: {
          bg: '#EFF6FF',
          border: '#BFDBFE',
        },
      },
      boxShadow: {
        card: '0 1px 2px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.02)',
        btn: '0 1px 2px rgba(0,0,0,0.06), inset 0 1px 0 rgba(255,255,255,0.10)',
        sticky:
          '0 10px 30px -8px rgba(0,0,0,0.15), 0 0 0 1px #ECECEE',
      },
    },
  },
  plugins: [],
};
