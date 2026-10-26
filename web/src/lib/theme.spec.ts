// The theme must keep text at WCAG AA (4.5:1) on every surface.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8');
const vars = Object.fromEntries(
	[...css.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6});/gi)].map((m) => [m[1], m[2]])
);

function luminance(hex: string): number {
	const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
	const [r, g, b] = c.map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: string, b: string): number {
	const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	return (hi + 0.05) / (lo + 0.05);
}

describe('theme contrast', () => {
	const surfaces = ['background', 'card', 'popover', 'secondary'];
	const texts = ['foreground', 'muted-foreground', 'primary', 'destructive', 'warning', 'success'];
	for (const s of surfaces) {
		for (const t of texts) {
			it(`${t} on ${s} ≥ 4.5`, () => {
				expect(contrast(vars[t], vars[s])).toBeGreaterThanOrEqual(4.5);
			});
		}
	}
	it('primary button text ≥ 4.5', () => {
		expect(contrast(vars['primary-foreground'], vars.primary)).toBeGreaterThanOrEqual(4.5);
	});
	it('surfaces are charcoal, not black', () => {
		expect(luminance(vars.background)).toBeGreaterThan(luminance('#1c1c1e'));
		expect(luminance(vars.card)).toBeGreaterThan(luminance(vars.background));
	});
});
