import path from "path";
import {defineConfig} from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The dev server stands in for the Go backend: every path the backend owns is
// proxied to it, so the frontend runs against real APIs without CORS or a
// separate ServerUrl. Anything else falls through to Vite and the SPA.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {"@": path.resolve(__dirname, "./src")},
  },
  server: {
    port: 8002,
    proxy: {
      "/api": {target: "http://localhost:9000", changeOrigin: true, ws: true},
      "/k8s": {target: "http://localhost:9000", changeOrigin: true, ws: true},
      "/.well-known": {target: "http://localhost:9000", changeOrigin: true},
    },
  },
  build: {
    outDir: "build",
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // echarts and xterm are only needed by a handful of routes; splitting
        // them keeps the initial bundle from carrying the whole charting stack.
        manualChunks: {
          echarts: ["echarts"],
          xterm: ["xterm", "xterm-addon-fit"],
          vendor: ["react", "react-dom", "react-router-dom"],
        },
      },
    },
  },
});
