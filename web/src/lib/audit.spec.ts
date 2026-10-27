import { describe, expect, it } from 'vitest';
import {
	auditLook,
	CATEGORY_FILTERS,
	dayLabel,
	detailEntries,
	groupByDay,
	parseActor
} from './audit';

describe('auditLook', () => {
	it('labels, categorizes and tones known actions by outcome', () => {
		const a = auditLook('grant.authorized');
		expect([a.label, a.category, a.tone]).toEqual(['Authorized', 'grant', 'success']);
		expect(auditLook('grant.denied').tone).toBe('danger');
		expect(auditLook('grant.requested').tone).toBe('warning');
		expect(auditLook('host.key_changed_detected').tone).toBe('danger');
		expect(auditLook('admin.login').category).toBe('session');
		expect(auditLook('ca.generated').category).toBe('system');
	});
	it('falls back to the raw action', () => {
		expect(auditLook('grant.something_new').label).toBe('grant.something_new');
		expect(auditLook('grant.something_new').category).toBe('grant');
	});
	it('every category filter prefix matches at least one known action', () => {
		for (const f of CATEGORY_FILTERS.filter((f) => f.prefix)) {
			expect(auditLook(`${f.prefix}x`).category).not.toBe('system');
		}
	});
});

describe('parseActor', () => {
	it('reads admin, agent and system actors', () => {
		expect(parseActor('admin:iPhone')).toEqual({
			kind: 'admin',
			name: 'iPhone',
			label: 'You (iPhone)'
		});
		expect(parseActor('agent:claude')).toEqual({ kind: 'agent', name: 'claude', label: 'claude' });
		expect(parseActor('system').label).toBe('Lor');
		expect(parseActor('console').kind).toBe('other');
	});
});

describe('detailEntries', () => {
	it('skips empty values and flattens arrays and objects', () => {
		expect(
			detailEntries({ level: 'sudo', note: '', lan_addrs: ['a', 'b'], x: { y: 1 }, n: null, z: 0 })
		).toEqual([
			['level', 'sudo'],
			['lan addrs', 'a, b'],
			['x', '{"y":1}'],
			['z', '0']
		]);
	});
});

describe('day grouping', () => {
	const now = new Date(2026, 9, 26, 15, 0).getTime();
	const at = (d: number, h: number) => new Date(2026, 9, d, h, 0).toISOString();
	it('labels today, yesterday and older days', () => {
		expect(dayLabel(now - 3600_000, now)).toBe('Today');
		expect(dayLabel(new Date(2026, 9, 25, 9).getTime(), now)).toBe('Yesterday');
		expect(dayLabel(new Date(2025, 0, 3).getTime(), now)).toContain('2025');
	});
	it('groups newest-first events by day, keeping order', () => {
		const groups = groupByDay(
			[{ ts: at(26, 14) }, { ts: at(26, 9) }, { ts: at(25, 20) }, { ts: at(20, 8) }],
			now
		);
		expect(groups.map((g) => [g.label, g.events.length])).toEqual([
			['Today', 2],
			['Yesterday', 1],
			[dayLabel(new Date(2026, 9, 20, 8).getTime(), now), 1]
		]);
	});
});
