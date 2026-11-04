/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,js,ts}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        canvas: {
          dark: '#222831',
          light: '#F9F7F7',
        },
        surface: {
          dark: '#393E46',
          light: '#FFFFFF',
        },
        border: {
          dark: '#4B5360',
          light: '#DBE2EF',
        },
        primary: {
          dark: '#00ADB5',
          light: '#3F72AF',
        },
        content: {
          dark: '#EEEEEE',
          muted: '#CBD5E1',
          light: '#112D4E',
          'muted-light': '#4B5563',
        },
        status: {
          success: '#10B981',
          warning: '#F59E0B',
          danger: '#F87171',
          info: '#38BDF8',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Geist Mono', 'monospace'],
      },
    },
  },
  plugins: [],
};
