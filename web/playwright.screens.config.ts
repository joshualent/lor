import { defineConfig, devices } from '@playwright/test';

// Screenshot harness (make screens): seeds realistic data and captures every
// page at desktop and phone widths against `lor-server serve --dev` plus the
// Vite dev server. Not part of CI.
export default defineConfig({
	testDir: '../test/screens',
	testMatch: '**/*.screens.ts',
	// Keep screenshots and E2E traces independent when both suites run.
	outputDir: '../test/screens/.tmp/results',
	globalSetup: '../test/screens/global-setup.ts',
	fullyParallel: false,
	workers: 1,
	reporter: 'list',
	use: {
		baseURL: 'http://localhost:15173',
		colorScheme: 'dark'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
