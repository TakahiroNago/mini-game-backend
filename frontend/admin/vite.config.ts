import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The admin UI is always served same-origin with the API: Vite proxies /api
// in development and nginx does the same in the container, so the browser
// never needs CORS.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  const target = env.VITE_API_PROXY_TARGET || "http://localhost:8080";
  return {
    plugins: [react(), tailwindcss()],
    server: {
      port: 5173,
      proxy: {
        "/api": { target, changeOrigin: true },
      },
    },
    build: { sourcemap: false },
  };
});
