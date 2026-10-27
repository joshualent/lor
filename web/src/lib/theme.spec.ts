// The theme must keep text at WCAG AA (4.5:1) on every surface it's used on,
// and non-text indicators (focus ring, decorative icons) at 3:1.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8');
const root = css.slice(css.indexOf(':root {'), css.indexOf('@theme inline'));
const vars: Record<string, string> = Object.fromEntries(
	[...root.matchAll(/--([\w-]+):\s*(#[0-9a-f]{6});/gi)].map((m) => [m[1], m[2].toLowerCase()])
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

function expectContrast(fg: string, bg: string, min: number) {
	expect(vars[fg], `--${fg} is defined`).toBeDefined();
	expect(vars[bg], `--${bg} is defined`).toBeDefined();
	const ratio = contrast(vars[fg], vars[bg]);
	expect(ratio, `--${fg} on --${bg} = ${ratio.toFixed(2)}`).toBeGreaterThanOrEqual(min);
}

// Every semantic tone is a triple: --<tone>, --<tone>-tint, --<tone>-edge.
const tones = Object.keys(vars)
	.filter((k) => k.endsWith('-tint'))
	.map((k) => k.slice(0, -'-tint'.length));

describe('theme tokens', () => {
	it('defines the expected semantic tones', () => {
		expect(tones.sort()).toEqual(
			['destructive', 'info', 'neutral', 'success', 'teal', 'violet', 'warning'].sort()
		);
		for (const t of tones) expect(vars[`${t}-edge`], `--${t}-edge`).toBeDefined();
	});

	it('every color the Tailwind theme maps exists in :root', () => {
		const theme = css.slice(css.indexOf('@theme inline'));
		const refs = [...theme.matchAll(/--color-[\w-]+:\s*var\(--([\w-]+)\)/g)].map((m) => m[1]);
		expect(refs.length).toBeGreaterThan(20);
		for (const r of refs) expect(root, `--${r}`).toContain(`--${r}:`);
	});
});

describe('text contrast (WCAG AA, 4.5:1)', () => {
	const surfaces = ['background', 'card', 'card-hover', 'popover', 'secondary'];
	const texts = ['foreground', 'muted-foreground', 'primary', ...tones];
	for (const s of surfaces) {
		for (const t of texts) {
			it(`${t} on ${s}`, () => expectContrast(t, s, 4.5));
		}
	}

	// Badges, callouts and icon tiles put the tone's text (or the main text)
	// on the tone's tint.
	for (const t of tones) {
		it(`${t} on ${t}-tint`, () => expectContrast(t, `${t}-tint`, 4.5));
		it(`foreground on ${t}-tint`, () => expectContrast('foreground', `${t}-tint`, 4.5));
	}

	it('primary button text', () => expectContrast('primary-foreground', 'primary', 4.5));
});

describe('non-text contrast (3:1)', () => {
	for (const s of ['background', 'card', 'popover']) {
		it(`focus ring on ${s}`, () => expectContrast('ring', s, 3));
		it(`muted icons on ${s}`, () => expectContrast('icon-muted', s, 3));
	}
	it('tone edges are visible against their tint', () => {
		for (const t of tones) {
			expect(contrast(vars[`${t}-edge`], vars[`${t}-tint`])).toBeGreaterThan(1.2);
		}
	});
});

describe('surfaces', () => {
	it('are charcoal, not black, and step up in lightness', () => {
		expect(luminance(vars.background)).toBeGreaterThan(luminance('#1c1c1e'));
		expect(luminance(vars.card)).toBeGreaterThan(luminance(vars.background));
		expect(luminance(vars['card-hover'])).toBeGreaterThan(luminance(vars.card));
		expect(luminance(vars.popover)).toBeGreaterThan(luminance(vars.card));
	});
	it('keeps the original base palette', () => {
		expect(vars.background).toBe('#232326');
		expect(vars.card).toBe('#2b2b2f');
		expect(vars.primary).toBe('#9db9ee');
	});
});
