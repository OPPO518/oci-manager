/** @type {import('tailwindcss').Config} */
// 色值只定义在 src/index.css 的 :root 中，这里全部通过 var(--rw-*) 引用，
// 消除双份色值；保留 oracle.* 旧键以兼容既有 className。
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        oracle: {
          // —— 旧键（兼容既有页面）——
          red: 'var(--rw-red)',
          redHover: 'var(--rw-brand-strong)',
          redSoft: 'var(--rw-selected)',
          ink: 'var(--rw-ink)',
          slate: 'var(--rw-cta)',
          text: 'var(--rw-ink-2)',
          border: 'var(--rw-border)',
          borderStrong: 'var(--rw-border)',
          surface: 'var(--rw-surface-2)',
          canvas: 'var(--rw-canvas)',
          muted: 'var(--rw-ink-3)',
          green: 'var(--rw-success)',
          amber: 'var(--rw-warning)',
          blue: 'var(--rw-info)',
          // —— Redwood 语义键 ——
          ink2: 'var(--rw-ink-2)',
          ink3: 'var(--rw-ink-3)',
          surface2: 'var(--rw-surface-2)',
          brand: 'var(--rw-brand)',
          brandStrong: 'var(--rw-brand-strong)',
          brandText: 'var(--rw-brand-text)',
          focus: 'var(--rw-focus)',
          cta: 'var(--rw-cta)',
          topbar: 'var(--rw-topbar)',
          selected: 'var(--rw-selected)',
          selectedBorder: 'var(--rw-selected-border)',
          success: 'var(--rw-success)',
          successBg: 'var(--rw-success-bg)',
          warning: 'var(--rw-warning)',
          warningBg: 'var(--rw-warning-bg)',
          danger: 'var(--rw-danger)',
          dangerBg: 'var(--rw-danger-bg)',
          info: 'var(--rw-info)',
          infoBg: 'var(--rw-info-bg)',
          pine: 'var(--rw-pine-100)',
          yellow: 'var(--rw-yellow-170)',
        },
      },
      fontFamily: {
        sans: [
          '"Public Sans"',
          '-apple-system',
          'BlinkMacSystemFont',
          '"Segoe UI"',
          '"Helvetica Neue"',
          'Arial',
          '"PingFang SC"',
          '"Microsoft YaHei"',
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
