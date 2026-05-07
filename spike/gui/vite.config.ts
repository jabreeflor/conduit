import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Vite is configured to talk to Tauri 2.x: the dev server listens on
// 1420 (not 5173) so tauri.conf.json can point at it without a clash.
export default defineConfig({
  plugins: [react()],
  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
  },
  envPrefix: ["VITE_", "TAURI_"],
  build: {
    target: "safari14",
    minify: "esbuild",
    sourcemap: true,
  },
});
