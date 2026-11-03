/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Oracle Redwood 风格配色
        oracle: {
          red: '#C74634',
          redHover: '#A83A2A',
          ink: '#161616',
          slate: '#312D2A',
          border: '#E4E4E3',
          surface: '#F7F7F5',
          muted: '#6B6B6B',
        },
      },
      fontFamily: {
        sans: ['"Oracle Sans"', 'Inter', 'system-ui', '-apple-system', 'Segoe UI', 'sans-serif'],
      },
      borderRadius: {
        DEFAULT: '4px',
      },
    },
  },
  plugins: [],
}
