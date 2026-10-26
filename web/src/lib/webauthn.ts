// Passkey ceremonies. Step-up sends the exact action to /stepup/begin; the
// server stores it with a one-time challenge and, after the tap, /stepup/finish
// (which carries only the assertion) applies exactly the stored action.
import {
	startAuthentication,
	startRegistration,
	type PublicKeyCredentialCreationOptionsJSON,
	type PublicKeyCredentialRequestOptionsJSON
} from '@simplewebauthn/browser';
import { api, call, type StepUpAction, type StepUpResult } from './api/client';

export async function setupPasskey(code: string, passkeyName: string) {
	const begin = await call(api.POST('/setup/begin', { body: { code, passkey_name: passkeyName } }));
	const credential = await startRegistration({
		optionsJSON: begin.options as unknown as PublicKeyCredentialCreationOptionsJSON
	});
	return call(
		api.POST('/setup/finish', {
			body: { ceremony_id: begin.ceremony_id, credential: credential as never }
		})
	);
}

export async function loginWithPasskey() {
	const begin = await call(api.POST('/login/begin', { body: {} }));
	const credential = await startAuthentication({
		optionsJSON: begin.options as unknown as PublicKeyCredentialRequestOptionsJSON
	});
	return call(
		api.POST('/login/finish', {
			body: { ceremony_id: begin.ceremony_id, credential: credential as never }
		})
	);
}

/** Runs a step-up: validate + store the action, passkey tap, apply. */
export async function stepUp(action: StepUpAction): Promise<StepUpResult> {
	const begin = await call(api.POST('/stepup/begin', { body: { action } }));
	const credential = await startAuthentication({
		optionsJSON: begin.options as unknown as PublicKeyCredentialRequestOptionsJSON
	});
	return call(
		api.POST('/stepup/finish', {
			body: { ceremony_id: begin.ceremony_id, credential: credential as never }
		})
	);
}

/** Adds a passkey: an add_passkey step-up, then the registration. */
export async function addPasskey(name: string) {
	const res = await stepUp({ kind: 'add_passkey', passkey_name: name });
	if (!res.registration) throw new Error('Lor did not return registration options');
	const credential = await startRegistration({
		optionsJSON: res.registration.options as unknown as PublicKeyCredentialCreationOptionsJSON
	});
	return call(
		api.POST('/passkeys/register/finish', {
			body: { ceremony_id: res.registration.ceremony_id, credential: credential as never }
		})
	);
}
