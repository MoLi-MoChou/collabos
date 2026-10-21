import { config } from "@collabos/eslint-config";
export default [
  ...config({ i18n: true }),
  {
    files: ["src/**/*.{ts,tsx}"],
    rules: {
      "no-restricted-imports": ["error", { patterns: ["@tanstack/react-router", "@tauri-apps/*", "zustand"] }],
    },
  },
];
