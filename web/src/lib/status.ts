// How domain states look: label + tone (+ icon). The single place that maps
// grant, extension, level, host and agent states to the design system's
// semantic tones (see docs/design-system.md). Pure, unit-tested.
import type { Component } from 'svelte';
import Server from '@lucide/svelte/icons/server';
import ServerOff from '@lucide/svelte/icons/server-off';
import Layers from '@lucide/svelte/icons/layers';
import ShieldAlert from '@lucide/svelte/icons/shield-alert';
import Bot from '@lucide/svelte/icons/bot';
import type { Tone } from './design/variants';

export interface Look {
	label: string;
	tone: Tone;
}

const GRANT: Record<string, Look> = {
	pending: { label: 'Needs approval', tone: 'warning' },
	offered: { label: 'Offered', tone: 'info' },
	active: { label: 'Active', tone: 'success' },
	denied: { label: 'Denied', tone: 'danger' },
	revoked: { label: 'Revoked', tone: 'danger' },
	expired: { label: 'Expired', tone: 'neutral' },
	released: { label: 'Released', tone: 'neutral' },
	request_timeout: { label: 'Timed out', tone: 'neutral' },
	offer_expired: { label: 'Offer expired', tone: 'neutral' }
};

const EXTENSION: Record<string, Look> = {
	pending: { label: 'Needs approval', tone: 'warning' },
	approved: { label: 'Approved', tone: 'success' },
	denied: { label: 'Denied', tone: 'danger' },
	request_timeout: { label: 'Timed out', tone: 'neutral' },
	moot: { label: 'No longer needed', tone: 'neutral' }
};

export function grantLook(status: string): Look {
	return GRANT[status] ?? { label: status, tone: 'neutral' };
}

export function extensionLook(status: string): Look {
	return EXTENSION[status] ?? { label: status, tone: 'neutral' };
}

const EXPIRING_SOON_MS = 5 * 60_000;
const EXPIRING_NOW_MS = 60_000;

/** Countdown tone: neutral → warning in the last 5 min → danger in the last minute. */
export function remainingTone(endMs: number, nowMs: number): Tone {
	const left = endMs - nowMs;
	if (left <= 0) return 'neutral';
	if (left <= EXPIRING_NOW_MS) return 'danger';
	if (left <= EXPIRING_SOON_MS) return 'warning';
	return 'neutral';
}

/** Fraction of the grant's window still left (0..1), for the time bar. */
export function remainingFraction(startMs: number, endMs: number, nowMs: number): number {
	const total = endMs - startMs;
	if (total <= 0) return 0;
	return Math.min(1, Math.max(0, (endMs - nowMs) / total));
}

// ------------------------------------------------------------------ hosts

export interface HostFlags {
	hypervisor: boolean;
	lor_infra: boolean;
	disabled?: boolean;
	/** Only full Host objects carry the key; HostRef (inside a grant) doesn't. */
	host_key?: string;
}

export interface HostLook {
	icon: Component;
	tone: Tone;
	/** Badges worth noticing: risk and problems only, not healthy defaults. */
	badges: Look[];
}

/** `full`: h is a Host (so a missing host_key means none was scanned yet). */
export function hostLook(h: HostFlags, full = false): HostLook {
	const badges: Look[] = [];
	if (h.lor_infra) badges.push({ label: 'Lor infra', tone: 'danger' });
	if (h.hypervisor) badges.push({ label: 'Hypervisor', tone: 'warning' });
	if (h.disabled) badges.push({ label: 'Disabled', tone: 'neutral' });
	else if (full && !h.host_key) badges.push({ label: 'No host key', tone: 'warning' });
	let icon: Component = Server;
	let tone: Tone = 'neutral';
	if (h.disabled) icon = ServerOff;
	else if (h.lor_infra) {
		icon = ShieldAlert;
		tone = 'danger';
	} else if (h.hypervisor) {
		icon = Layers;
		tone = 'warning';
	}
	return { icon, tone, badges };
}

// ----------------------------------------------------------------- agents

export interface AgentFlags {
	status: string;
	enrolled: boolean;
}

/** Only states that need attention get a badge; a healthy agent has none. */
export function agentLook(a: AgentFlags): { icon: Component; tone: Tone; badge: Look | null } {
	if (a.status === 'disabled')
		return { icon: Bot, tone: 'neutral', badge: { label: 'Disabled', tone: 'neutral' } };
	if (!a.enrolled)
		return { icon: Bot, tone: 'warning', badge: { label: 'Not enrolled', tone: 'warning' } };
	return { icon: Bot, tone: 'neutral', badge: null };
}

/** "admin:iPhone" → "iPhone", for "decided by". */
export function passkeyName(actor: string | undefined): string {
	return (actor ?? '').replace(/^admin:/, '');
}
