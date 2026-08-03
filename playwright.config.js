const { defineConfig } = require("@playwright/test");

const go = process.env.GO || "go";
const port = process.env.E2E_PORT || "18080";
const baseURL = `http://127.0.0.1:${port}`;

module.exports = defineConfig({
  testDir: "./e2e",
  outputDir: "output/playwright/test-results",
  timeout: 45_000,
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: `ADDR=127.0.0.1:${port} CGO_ENABLED=0 ${go} run ./cmd/server`,
    url: `${baseURL}/healthz`,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
