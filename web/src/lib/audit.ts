// How audit events look: a human label, a category (icon), an outcome tone,
// who did it, and day grouping for the log. Pure, unit-tested.
import type { Component } from 'svelte';
import KeyRound from '@lucide/svelte/icons/key-round';
import CircleCheck from '@lucide/svelte/icons/circle-check';
import CircleX from '@lucide/svelte/icons/circle-x';
import CircleDashed from '@lucide/svelte/icons/circle-dashed';
import Hourglass from '@lucide/svelte/icons/hourglass';
import Send from '@lucide/svelte/icons/send';
import Inbox from '@lucide/svelte/icons/inbox';
import TimerReset from '@lucide/svelte/icons/timer-reset';
import FileBadge from '@lucide/svelte/icons/file-badge';
import Bot from '@lucide/svelte/icons/bot';
import Server from '@lucide/svelte/icons/server';
import Fingerprint from '@lucide/svelte/icons/fingerprint';
import LogIn from '@lucide/svelte/icons/log-in';
import LogOut from '@lucide/svelte/icons/log-out';
import ShieldCheck from '@lucide/svelte/icons/shield-check';
import ShieldAlert from '@lucide/svelte/icons/shield-alert';
import Undo2 from '@lucide/svelte/icons/undo-2';
import Repeat from '@lucide/svelte/icons/repeat';
import Download from '@lucide/svelte/icons/download';
import type { Tone } from './design/variants';

export type AuditCategory =
	'grant' | 'extension' | 'cert' | 'agent' | 'host' | 'passkey' | 'session' | 'system';

export interface AuditLook {
	label: string;
	category: AuditCategory;
	tone: Tone;
	icon: Component;
}

// label, tone, icon. The tone is the outcome: success = access given,
// warning = waiting for you / needs care, danger = refused or ended by you
// or a security event, neutral = routine.
const ACTIONS: Record<string, [string, Tone, Component]> = {
	'grant.requested': ['Requested', 'warning', Inbox],
	'grant.offered': ['Offer created', 'info', Send],
	'grant.offer_claimed': ['Offer claimed', 'success', CircleCheck],
	'grant.authorized': ['Authorized', 'success', CircleCheck],
	'grant.denied': ['Denied', 'danger', CircleX],
	'grant.extended': ['Extended', 'info', TimerReset],
	'grant.released': ['Released by the agent', 'neutral', Undo2],
	'grant.revoked': ['Revoked', 'danger', CircleX],
	'grant.expired': ['Expired', 'neutral', Hourglass],
	'grant.offer_expired': ['Offer expired', 'neutral', Hourglass],
	'grant.request_timeout': ['Request timed out', 'neutral', Hourglass],
	'grant.reused': ['Reused by a new request', 'neutral', Repeat],
	'cert.issued': ['Certificate issued', 'teal', FileBadge],
	'extension.requested': ['Extension requested', 'warning', TimerReset],
	'extension.approved': ['Extension approved', 'success', TimerReset],
	'extension.denied': ['Extension denied', 'danger', TimerReset],
	'extension.request_timeout': ['Extension timed out', 'neutral', Hourglass],
	'extension.moot': ['Extension no longer needed', 'neutral', CircleDashed],
	'agent.created': ['Agent added', 'violet', Bot],
	'agent.enrolled': ['Agent enrolled', 'violet', Bot],
	'agent.token_rotated': ['Agent token rotated', 'violet', KeyRound],
	'agent.disabled': ['Agent disabled', 'neutral', Bot],
	'agent.enabled': ['Agent enabled', 'violet', Bot],
	'agent.addrs_changed': ['Agent LAN IPs changed', 'violet', Bot],
	'host.created': ['Host added', 'teal', Server],
	'host.updated': ['Host updated', 'teal', Server],
	'host.key_scanned': ['Host key scanned', 'teal', Fingerprint],
	'host.key_changed_detected': ['Changed host key detected', 'danger', ShieldAlert],
	'host.key_changed_accepted': ['Changed host key accepted', 'warning', Fingerprint],
	'host.bootstrap_fetched': ['Bootstrap script fetched', 'teal', Download],
	'passkey.added': ['Passkey added', 'neutral', KeyRound],
	'passkey.removed': ['Passkey removed', 'neutral', KeyRound],
	'passkey.renamed': ['Passkey renamed', 'neutral', KeyRound],
	'admin.setup': ['First passkey registered', 'neutral', KeyRound],
	'admin.login': ['Signed in', 'neutral', LogIn],
	'admin.logout': ['Signed out', 'neutral', LogOut],
	'admin.sessions_revoked': ['Signed out everywhere', 'warning', LogOut],
	'admin.reset': ['Admin reset (console)', 'danger', ShieldAlert],
	'ca.generated': ['CA key generated', 'neutral', ShieldCheck],
	'tls.generated': ['TLS certificate generated', 'neutral', ShieldCheck]
};

const PREFIX_CATEGORY: Record<string, AuditCategory> = {
	grant: 'grant',
	extension: 'extension',
	cert: 'cert',
	agent: 'agent',
	host: 'host',
	passkey: 'passkey',
	admin: 'session'
};

export function auditLook(action: string): AuditLook {
	const category = PREFIX_CATEGORY[action.split('.')[0]] ?? 'system';
	const known = ACTIONS[action];
	if (known) return { label: known[0], tone: known[1], icon: known[2], category };
	return { label: action, tone: 'neutral', icon: CircleDashed, category };
}

/** Category filters for the log (the API filters by one action prefix). */
export const CATEGORY_FILTERS: { label: string; prefix: string }[] = [
	{ label: 'All', prefix: '' },
	{ label: 'Grants', prefix: 'grant.' },
	{ label: 'Extensions', prefix: 'extension.' },
	{ label: 'Certificates', prefix: 'cert.' },
	{ label: 'Hosts', prefix: 'host.' },
	{ label: 'Agents', prefix: 'agent.' },
	{ label: 'Passkeys', prefix: 'passkey.' },
	{ label: 'Sign-ins', prefix: 'admin.' }
];

/** Actor filters (the API matches an actor prefix). */
export const ACTOR_FILTERS: { label: string; value: string }[] = [
	{ label: 'Everyone', value: '' },
	{ label: 'You', value: 'admin:' },
	{ label: 'Agents', value: 'agent:' },
	{ label: 'System', value: 'system' }
];

export interface Actor {
	kind: 'admin' | 'agent' | 'system' | 'other';
	/** Passkey name, agent name, or the raw actor. */
	name: string;
	/** Short phrase: "You (iPhone)", "claude", "Lor". */
	label: string;
}

export function parseActor(actor: string): Actor {
	if (actor.startsWith('admin:')) {
		const name = actor.slice('admin:'.length);
		return { kind: 'admin', name, label: name ? `You (${name})` : 'You' };
	}
	if (actor === 'admin') return { kind: 'admin', name: '', label: 'You' };
	if (actor.startsWith('agent:')) {
		const name = actor.slice('agent:'.length);
		return { kind: 'agent', name, label: name };
	}
	if (actor === 'system') return { kind: 'system', name: 'system', label: 'Lor' };
	return { kind: 'other', name: actor, label: actor };
}

/** Event details as display pairs, skipping empty values. */
export function detailEntries(d: Record<string, unknown>): [string, string][] {
	return Object.entries(d)
		.filter(([, v]) => v !== '' && v !== null && v !== undefined)
		.map(([k, v]) => [
			k.replace(/_/g, ' '),
			Array.isArray(v) ? v.join(', ') : typeof v === 'object' ? JSON.stringify(v) : String(v)
		]);
}

function dayKey(ms: number): string {
	const d = new Date(ms);
	return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

/** "Today", "Yesterday", "Monday, Oct 20", or "Oct 3, 2025" for another year. */
export function dayLabel(ms: number, nowMs: number): string {
	if (dayKey(ms) === dayKey(nowMs)) return 'Today';
	if (dayKey(ms) === dayKey(nowMs - 86_400_000)) return 'Yesterday';
	const d = new Date(ms);
	const n = new Date(nowMs);
	if (d.getFullYear() !== n.getFullYear())
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
	if (nowMs - ms < 7 * 86_400_000)
		return d.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
	return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/** Groups newest-first events by local day, keeping their order. */
export function groupByDay<T extends { ts: string }>(
	events: T[],
	nowMs: number
): { key: string; label: string; events: T[] }[] {
	const groups: { key: string; label: string; events: T[] }[] = [];
	for (const ev of events) {
		const ms = Date.parse(ev.ts);
		const key = dayKey(ms);
		const last = groups.at(-1);
		if (last && last.key === key) last.events.push(ev);
		else groups.push({ key, label: dayLabel(ms, nowMs), events: [ev] });
	}
	return groups;
}
