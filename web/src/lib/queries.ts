// Query keys and fetchers for svelte-query. Mutations invalidate by prefix.
import { api, call } from './api/client';
import type { components } from './api/admin';

type S = components['schemas'];

export const keys = {
	session: ['session'] as const,
	dashboard: ['dashboard'] as const,
	grant: (id: number) => ['grant', id] as const,
	grants: ['grant'] as const,
	extension: (id: number) => ['extension', id] as const,
	hosts: ['hosts'] as const,
	host: (name: string) => ['hosts', name] as const,
	agents: ['agents'] as const,
	audit: (f: Record<string, string | number | undefined>) => ['audit', f] as const,
	passkeys: ['passkeys'] as const,
	sessions: ['sessions'] as const
};

/** Live pages poll every 5 seconds (and refetch on focus by default). */
export const LIVE_MS = 5000;

export const fetchSession = () => call(api.GET('/session'));
export const fetchDashboard = () => call(api.GET('/dashboard'));
export const fetchGrant = (id: number) =>
	call(api.GET('/grants/{id}', { params: { path: { id } } }));
export const fetchExtension = (id: number) =>
	call(api.GET('/extensions/{id}', { params: { path: { id } } }));
export const fetchHosts = () => call(api.GET('/hosts')).then((r) => r.hosts);
export const fetchHost = (name: string) =>
	call(api.GET('/hosts/{name}', { params: { path: { name } } }));
export const fetchAgents = () => call(api.GET('/agents')).then((r) => r.agents);
export const fetchPasskeys = () => call(api.GET('/passkeys')).then((r) => r.passkeys);
export const fetchSessions = () => call(api.GET('/sessions')).then((r) => r.sessions);
export const fetchAudit = (q: {
	actor?: string;
	action?: string;
	grant_id?: number;
	host_id?: number;
	agent_id?: number;
	before?: number;
}) => call(api.GET('/audit', { params: { query: { ...q, limit: 100 } } })).then((r) => r.events);

export type Grant = S['GrantDetail'];
export type Summary = S['GrantSummary'];
export type Host = S['Host'];
export type Agent = S['Agent'];
export type AuditEvent = S['AuditEvent'];
