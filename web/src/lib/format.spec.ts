import { describe, expect, it } from 'vitest';
import { formatArgv, formatCountdown, formatDuration, formatAgo } from './format';

describe('formatDuration', () => {
	it.each([
		[900, '15 min'],
		[3600, '1 h'],
		[5400, '1 h 30 min'],
		[86400, '1 day'],
		[3 * 86400, '3 days'],
		[604800, '1 week'],
		[2 * 604800, '2 weeks'],
		[90000, '1 d 1 h'],
		[45, '45 s']
	])('%i s → %s', (s, want) => {
		expect(formatDuration(s)).toBe(want);
	});
});

describe('formatCountdown', () => {
	const now = 1_000_000_000_000;
	it.each([
		[2 * 86400 + 4 * 3600 + 5, '2 d 4 h'],
		[3 * 3600 + 5 * 60 + 30, '3 h 05 min'],
		[12 * 60 + 30, '12 min'],
		[4 * 60 + 9, '4:09'],
		[7, '0:07'],
		[0, 'ended'],
		[-30, 'ended']
	])('%i s left → %s', (left, want) => {
		expect(formatCountdown(now + left * 1000, now)).toBe(want);
	});
	it('ticks every second near the end', () => {
		expect(formatCountdown(now + 61_000, now)).toBe('1:01');
		expect(formatCountdown(now + 61_000, now + 1000)).toBe('1:00');
	});
});

describe('formatAgo', () => {
	const now = 1_000_000_000_000;
	it('formats relative times', () => {
		expect(formatAgo(now - 10_000, now)).toBe('just now');
		expect(formatAgo(now - 120_000, now)).toBe('2 min ago');
		expect(formatAgo(now - 3 * 3600_000, now)).toBe('3 h ago');
	});
});

describe('formatArgv', () => {
	it('quotes like a shell', () => {
		expect(formatArgv(['zpool', 'status', '-v'])).toBe('zpool status -v');
		expect(formatArgv(['bash', '-c', "echo 'hi' && ls"])).toBe(`bash -c 'echo '\\''hi'\\'' && ls'`);
		expect(formatArgv([])).toBe('(interactive shell)');
		expect(formatArgv(['<script>'])).toBe("'<script>'");
	});
});
