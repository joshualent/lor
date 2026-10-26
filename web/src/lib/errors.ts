// Maps errors to the text shown in toasts. The server's message is shown as
// is; only a few codes get friendlier wording or a hint.
import { ApiError } from './api/error';

const HINTS: Record<string, string> = {
	ceremony_expired: 'The passkey prompt expired. Try again.',
	ceremony_not_found: 'That passkey prompt was already used. Try again.',
	passkey_failed: "The passkey couldn't be verified.",
	state_changed: 'This changed in the meantime. The page has been refreshed.',
	network: "Can't reach Lor. Check your connection and try again."
};

export function errorMessage(err: unknown): string {
	if (err instanceof ApiError) {
		return HINTS[err.code] ?? err.message;
	}
	if (err instanceof Error) {
		// WebAuthn cancellations from the browser.
		if (err.name === 'NotAllowedError' || err.name === 'AbortError') {
			return 'Passkey prompt was cancelled.';
		}
		if (err.name === 'InvalidStateError') {
			return 'This passkey is already registered.';
		}
		return err.message || 'Something went wrong.';
	}
	return 'Something went wrong.';
}

/** True when the user just dismissed the passkey prompt (no error toast needed). */
export function isCancel(err: unknown): boolean {
	return err instanceof Error && (err.name === 'NotAllowedError' || err.name === 'AbortError');
}
