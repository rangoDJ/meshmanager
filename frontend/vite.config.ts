import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    // In development the Go server runs on :8080 and serves only the API.
    proxy: { "/api": "http://localhost:8080" },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
