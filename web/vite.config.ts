import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// emptyOutDir 会清空 Go 的 embed 目录，连带删掉仓库里用于保留空目录的
// .gitkeep；每次构建后重新生成它，避免 git status 反复出现误删、以及干净
// clone 后未构建前端时 go:embed 找不到目录。
const keepEmbedDir: Plugin = {
  name: 'keep-embed-dir-gitkeep',
  generateBundle() {
    this.emitFile({ type: 'asset' as const, fileName: '.gitkeep', source: '' })
  },
}

export default defineConfig({
  plugins: [react(), keepEmbedDir],
  build: {
    // 直接输出到 Go 的 embed 目录，构建后二进制自带前端
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
})
