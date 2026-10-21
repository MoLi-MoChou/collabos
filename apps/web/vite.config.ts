import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwind from "@tailwindcss/vite";

const target = process.env.COLLAB_API ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwind()],
  server: {
    port: 5173,
    proxy: { "/api": target, "/ws": { target, ws: true } },
  },
});
