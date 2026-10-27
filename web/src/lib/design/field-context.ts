// Lets a Field or SettingsRow describe its control without every page
// wiring ids by hand: the control (Input, Textarea, NativeSelect, Switch)
// picks up aria-describedby / aria-invalid when its id matches.
import { getContext, setContext } from 'svelte';

const KEY = Symbol('lor-field');

export interface FieldContext {
	readonly id: string | undefined;
	readonly describedBy: string | undefined;
	readonly invalid: boolean;
}

export function setFieldContext(ctx: FieldContext) {
	setContext(KEY, ctx);
}

/**
 * Call during component initialization. Returns a function giving the aria
 * attributes for the control with a given id (empty outside a field).
 */
export function useFieldAria(): (id: unknown) => {
	'aria-describedby'?: string;
	'aria-invalid'?: true;
} {
	const ctx = getContext<FieldContext | undefined>(KEY);
	return (id) => {
		if (!ctx || !ctx.id || ctx.id !== id) return {};
		return {
			...(ctx.describedBy ? { 'aria-describedby': ctx.describedBy } : {}),
			...(ctx.invalid ? { 'aria-invalid': true as const } : {})
		};
	};
}
