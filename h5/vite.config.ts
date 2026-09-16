import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import {defineConfig} from 'vite';

export default defineConfig(({ command }) => {
  return {
    // 生产构建部署在 console.zerone.life/static/h5/ 下（agent-hub 静态目录的 h5 子目录），
    // 资源路径必须以 /static/h5/ 为前缀；dev 服务器保持根路径。
    // 部署到别处时用 VITE_BASE 覆盖：VITE_BASE=/ npm run build
    base: process.env.VITE_BASE ?? (command === 'build' ? '/static/h5/' : '/'),
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, '.'),
      },
    },
    server: {
      // HMR is disabled in AI Studio via DISABLE_HMR env var.
      // Do not modifyâfile watching is disabled to prevent flickering during agent edits.
      hmr: process.env.DISABLE_HMR !== 'true',
      // Disable file watching when DISABLE_HMR is true to save CPU during agent edits.
      watch: process.env.DISABLE_HMR === 'true' ? null : {},
    },
  };
});
