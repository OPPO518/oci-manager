/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Oracle Redwood 风格配色
        oracle: {
          red: '#C74634',
          redHover: '#A4372A',
          redSoft: '#FBEEEB',
          ink: '#312D2A',
          slate: '#312D2A',
          text: '#3D3A37',
          border: '#DEDCD9',
          borderStrong: '#C9C6C2',
          surface: '#F7F6F4',
          canvas: '#F4F3F1',
          muted: '#6B6763',
          green: '#1B7F4B',
          amber: '#B26A00',
          blue: '#1A6BA8',
        },
      },
      fontFamily: {
        sans: [
          '"Oracle Sans"',
          '"Helvetica Neue"',
          'Inter',
          'system-ui',
          '-apple-system',
          'Segoe UI',
          'Microsoft YaHei',
          'sans-serif',
        ],
      },
      borderRadius: {
        DEFAULT: '4px',
      },
    },
  },
  plugins: [],
}
