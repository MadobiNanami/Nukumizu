import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

// Backend the dev server proxies /api (and the log websocket) to.
const BACKEND = process.env.NUKUMIZU_API || 'http://127.0.0.1:8080';

export default defineConfig({
    plugins: [vue()],
    server: {
        host: '0.0.0.0',
        port: 5173,
        proxy: {
            '/api': {
                target: BACKEND,
                changeOrigin: true,
                ws: true
            }
        }
    }
});
