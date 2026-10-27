import { describe, expect, it } from 'vitest';
import {
	formatArgv,
	formatCountdown,
	formatDuration,
	formatAgo,
	formatClock,
	offerChatLine,
	formatTimeOfDay
} from './format';

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
	it('shows what the remote shell runs (arguments joined by spaces, like ssh)', () => {
		expect(formatArgv(['zpool', 'status', '-v'])).toBe('zpool status -v');
		expect(formatArgv(['zpool status | head'])).toBe('zpool status | head');
		expect(formatArgv(['<script>'])).toBe('<script>');
		expect(formatArgv([])).toBe('(interactive shell)');
	});
});

describe('time zones', () => {
	const end = Date.UTC(2026, 9, 26, 19, 51);
	it('labels clock times with the zone', () => {
		expect(formatClock(end, end, 'America/Detroit')).toMatch(/03:51\sPM EDT|15:51 EDT/);
		expect(formatClock(end, end, 'UTC')).toMatch(/UTC/);
	});
	it('gives the offer chat line in the admin zone and UTC', () => {
		expect(offerChatLine('vikunja', 'user', end, 'America/Detroit')).toBe(
			'You have Lor access to `vikunja` until 15:51 EDT (19:51 UTC). Use `lor ssh vikunja -- <cmd>`.'
		);
		expect(offerChatLine('nas', 'sudo', end, 'UTC')).toBe(
			'You have Lor access to `nas` (sudo) until 19:51 UTC. Use `lor ssh nas -- <cmd>`.'
		);
	});
});

describe('formatTimeOfDay', () => {
	it('shows hours and minutes only', () => {
		const t = formatTimeOfDay(new Date(2026, 9, 26, 14, 5, 59).getTime());
		expect(t).toMatch(/0?2:05|14:05/);
		expect(t).not.toMatch(/59/);
	});
});
