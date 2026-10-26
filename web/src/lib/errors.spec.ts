import { describe, expect, it } from 'vitest';
import { ApiError } from './api/error';
import { errorMessage, isCancel } from './errors';

describe('errorMessage', () => {
	it("shows the server's message", () => {
		expect(
			errorMessage(new ApiError(400, 'sudo_not_confirmed', 'confirm that this gives root on nas'))
		).toBe('confirm that this gives root on nas');
	});
	it('rewords a few codes', () => {
		expect(errorMessage(new ApiError(400, 'ceremony_expired', 'x'))).toMatch(/expired/);
		expect(errorMessage(new ApiError(0, 'network', 'x'))).toMatch(/reach Lor/);
	});
	it('handles WebAuthn cancellations', () => {
		const e = new Error('The operation either timed out or was not allowed.');
		e.name = 'NotAllowedError';
		expect(errorMessage(e)).toBe('Passkey prompt was cancelled.');
		expect(isCancel(e)).toBe(true);
		expect(isCancel(new Error('x'))).toBe(false);
	});
	it('falls back for unknown values', () => {
		expect(errorMessage('nope')).toBe('Something went wrong.');
	});
});
