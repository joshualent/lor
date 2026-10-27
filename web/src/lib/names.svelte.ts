// Host and agent names by id, for rows that only carry ids (audit events).
// Call during component initialization; the lists are small and cached.
import { createQuery } from '@tanstack/svelte-query';
import { keys, fetchAgents, fetchHosts } from './queries';

export interface Names {
	hosts: Record<number, string>;
	agents: Record<number, string>;
}

export function useNames(): { readonly current: Names } {
	const hosts = createQuery(() => ({ queryKey: keys.hosts, queryFn: fetchHosts }));
	const agents = createQuery(() => ({ queryKey: keys.agents, queryFn: fetchAgents }));
	const names = $derived({
		hosts: Object.fromEntries((hosts.data ?? []).map((h) => [h.id, h.name])),
		agents: Object.fromEntries((agents.data ?? []).map((a) => [a.id, a.name]))
	});
	return {
		get current() {
			return names;
		}
	};
}
