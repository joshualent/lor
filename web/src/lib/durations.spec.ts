import { describe, expect, it } from 'vitest';
import { DURATION_CHOICES, choiceFor, isLong, parseDuration, validDuration } from './durations';

describe('duration picker', () => {
	it('offers the plan presets in order', () => {
		expect(DURATION_CHOICES.map((c) => c.label)).toEqual([
			'15 min',
			'30 min',
			'1 h',
			'2 h',
			'4 h',
			'8 h',
			'1 day',
			'3 days',
			'1 week'
		]);
	});
	it('notes durations over 24h without blocking them', () => {
		expect(isLong(86400)).toBe(false);
		expect(isLong(86401)).toBe(true);
		expect(validDuration(7 * 86400, 60, 30 * 86400)).toBe(true);
	});
	it('maps seconds back to presets', () => {
		expect(choiceFor(3600)?.label).toBe('1 h');
		expect(choiceFor(4000)).toBeUndefined();
	});
});

describe('parseDuration', () => {
	it.each([
		['90m', 5400],
		['2h', 7200],
		['1h30m', 5400],
		['1h 30m', 5400],
		['3d', 259200],
		['1w', 604800],
		['45', 2700],
		['1.5h', 5400],
		['10min', 600]
	])('%s → %i', (input, want) => {
		expect(parseDuration(input)).toBe(want);
	});
	it.each(['', 'abc', '0', '-1h', 'h', '1x', '1h abc'])('rejects %j', (input) => {
		expect(parseDuration(input)).toBeNull();
	});
	it('enforces bounds', () => {
		expect(validDuration(30, 60, 3600)).toBe(false);
		expect(validDuration(7200, 60, 3600)).toBe(false);
		expect(validDuration(null, 60, 3600)).toBe(false);
	});
});
