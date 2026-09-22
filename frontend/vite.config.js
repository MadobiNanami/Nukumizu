import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

// Backend the dev server proxies /api (and the log websocket) to.
const BACKEND = process.env.NUKUMIZU_API || 'http://127.0.0.1:8080';

export default defineConfig({
    plugins: [vue()],
    build: {
        // web/embed.go embeds this directory into the Go binary. It has to live
        // inside the web package: go:embed cannot reach outside its own
        // directory, so ../web/dist is as close as it gets.
        outDir: '../web/dist',
        // The directory is outside the project root, so Vite refuses to empty
        // it unless told to. Without this, stale hashed assets from earlier
        // builds pile up in the binary.
        emptyOutDir: true
    },
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
