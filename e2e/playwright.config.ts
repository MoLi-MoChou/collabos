import { defineConfig } from "@playwright/test";

export const PORT = Number(process.env.E2E_PORT ?? 8092);

export default defineConfig({
  testDir: "tests",
  globalSetup: "./tests/global-setup.ts",
  globalTeardown: "./tests/global-teardown.ts",
  workers: 1,
  fullyParallel: false,
  timeout: 15 * 60_000,
  expect: { timeout: 20_000 },
  reporter: [["list"]],
  use: {
    baseURL: `http://localhost:${PORT}`,
    locale: "zh-CN",
    viewport: { width: 1440, height: 900 },
    launchOptions: { executablePath: process.env.CHROME_PATH ?? "/usr/bin/google-chrome" },
    trace: "retain-on-failure",
  },
});
