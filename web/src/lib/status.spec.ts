import { describe, expect, it } from 'vitest';
import {
	agentLook,
	extensionLook,
	grantLook,
	hostLook,
	passkeyName,
	remainingFraction,
	remainingTone
} from './status';

describe('grantLook', () => {
	it('maps every grant status to a tone that carries meaning', () => {
		expect(grantLook('pending')).toEqual({ label: 'Needs approval', tone: 'warning' });
		expect(grantLook('active').tone).toBe('success');
		expect(grantLook('offered').tone).toBe('info');
		expect(grantLook('denied').tone).toBe('danger');
		expect(grantLook('revoked').tone).toBe('danger');
		for (const s of ['expired', 'released', 'request_timeout', 'offer_expired'])
			expect(grantLook(s).tone).toBe('neutral');
	});
	it('falls back to the raw status', () => {
		expect(grantLook('weird')).toEqual({ label: 'weird', tone: 'neutral' });
	});
});

describe('extensionLook', () => {
	it('maps extension statuses', () => {
		expect(extensionLook('approved').tone).toBe('success');
		expect(extensionLook('moot')).toEqual({ label: 'No longer needed', tone: 'neutral' });
	});
});

describe('remainingTone', () => {
	const now = 1_000_000;
	it('turns warning in the last 5 minutes and danger in the last minute', () => {
		expect(remainingTone(now + 10 * 60_000, now)).toBe('neutral');
		expect(remainingTone(now + 4 * 60_000, now)).toBe('warning');
		expect(remainingTone(now + 30_000, now)).toBe('danger');
		expect(remainingTone(now - 1, now)).toBe('neutral');
	});
	it('computes the fraction left, clamped', () => {
		expect(remainingFraction(0, 100, 25)).toBe(0.75);
		expect(remainingFraction(0, 100, 200)).toBe(0);
		expect(remainingFraction(0, 100, -50)).toBe(1);
		expect(remainingFraction(5, 5, 5)).toBe(0);
	});
});

describe('hostLook', () => {
	it('badges risk and problems, not healthy defaults', () => {
		expect(hostLook({ hypervisor: false, lor_infra: false, host_key: 'k' }, true).badges).toEqual(
			[]
		);
		const infra = hostLook({ hypervisor: true, lor_infra: true, host_key: 'k' }, true);
		expect(infra.tone).toBe('danger');
		expect(infra.badges.map((b) => b.label)).toEqual(['Lor infra', 'Hypervisor']);
	});
	it('flags a missing host key only for full Host objects', () => {
		expect(hostLook({ hypervisor: false, lor_infra: false }, true).badges[0].label).toBe(
			'No host key'
		);
		expect(hostLook({ hypervisor: false, lor_infra: false }).badges).toEqual([]);
	});
	it('a disabled host says so instead of nagging about its key', () => {
		const h = hostLook({ hypervisor: false, lor_infra: false, disabled: true }, true);
		expect(h.badges.map((b) => b.label)).toEqual(['Disabled']);
	});
});

describe('agentLook', () => {
	it('badges only states that need attention', () => {
		expect(agentLook({ status: 'active', enrolled: true }).badge).toBeNull();
		expect(agentLook({ status: 'active', enrolled: false }).badge?.tone).toBe('warning');
		expect(agentLook({ status: 'disabled', enrolled: true }).badge?.label).toBe('Disabled');
	});
});

it('passkeyName strips the admin: prefix', () => {
	expect(passkeyName('admin:iPhone')).toBe('iPhone');
	expect(passkeyName(undefined)).toBe('');
});
