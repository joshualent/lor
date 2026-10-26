import { defineConfig, devices } from '@playwright/test';

// E2E tests live in test/e2e and run against `lor-server serve --dev`
// (started by test/e2e/global-setup.ts) with Chrome's virtual authenticator.
export default defineConfig({
	testDir: '../test/e2e',
	testMatch: '**/*.e2e.ts',
	globalSetup: '../test/e2e/global-setup.ts',
	fullyParallel: false,
	workers: 1,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
	use: {
		baseURL: 'http://localhost:18444',
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
