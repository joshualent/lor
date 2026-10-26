<script lang="ts">
	import { page } from '$app/state';
	import { createQuery } from '@tanstack/svelte-query';
	import { errorMessage } from '#lib/errors.js';
	import { formatTimestamp } from '#lib/format.js';
	import { keys, fetchAudit, type AuditEvent } from '#lib/queries.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import Section from '#lib/components/lor/Section.svelte';

	let actor = $state(page.url.searchParams.get('actor') ?? '');
	let action = $state(page.url.searchParams.get('action') ?? '');
	let grant = $state(page.url.searchParams.get('grant_id') ?? '');
	let older = $state<AuditEvent[]>([]);
	let loadingMore = $state(false);

	const filter = $derived({
		actor: actor || undefined,
		action: action.trim() || undefined,
		grant_id: Number(grant) || undefined
	});
	const q = createQuery(() => ({
		queryKey: keys.audit(filter),
		queryFn: () => fetchAudit(filter),
		refetchInterval: 15_000
	}));
	$effect(() => {
		void filter;
		older = [];
	});

	const events = $derived([...(q.data ?? []), ...older]);

	async function more() {
		const last = events.at(-1);
		if (!last) return;
		loadingMore = true;
		try {
			older = [...older, ...(await fetchAudit({ ...filter, before: last.id }))];
		} finally {
			loadingMore = false;
		}
	}

	function details(d: Record<string, unknown>): string {
		const parts = Object.entries(d).filter(([, v]) => v !== '' && v !== null && v !== undefined);
		return parts
			.map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : String(v)}`)
			.join(' ');
	}
</script>

<PageHeader title="Audit log" subtitle="Append-only. Newest first." />

<div class="mb-4 grid grid-cols-1 gap-2 sm:grid-cols-3">
	<select
		bind:value={actor}
		class="h-10 rounded-md border border-input bg-background px-3"
		aria-label="Actor"
	>
		<option value="">Everyone</option>
		<option value="admin:">Admin</option>
		<option value="agent:">Agents</option>
		<option value="system">System</option>
	</select>
	<Input bind:value={action} placeholder="Action, e.g. grant." aria-label="Action prefix" />
	<Input bind:value={grant} placeholder="Grant #" inputmode="numeric" aria-label="Grant ID" />
</div>

<Section>
	{#if q.isPending}
		<p class="text-muted-foreground">Loading…</p>
	{:else if q.isError}
		<p class="text-destructive">{errorMessage(q.error)}</p>
	{:else if events.length === 0}
		<p class="text-muted-foreground">No matching events.</p>
	{:else}
		<ol class="-my-2 divide-y divide-border text-sm">
			{#each events as ev (ev.id)}
				<li class="py-2.5">
					<p>
						<span class="font-medium">{ev.action}</span>
						<span class="text-muted-foreground">· {ev.actor}</span>
						{#if ev.grant_id}· <a href="/grants/{ev.grant_id}">grant #{ev.grant_id}</a>{/if}
					</p>
					<p class="text-muted-foreground tabular-nums">{formatTimestamp(Date.parse(ev.ts))}</p>
					{#if Object.keys(ev.details).length}
						<p class="mt-0.5 font-mono text-xs break-all text-muted-foreground">
							{details(ev.details)}
						</p>
					{/if}
				</li>
			{/each}
		</ol>
		{#if (q.data?.length ?? 0) >= 100}
			<Button variant="ghost" class="mt-3" disabled={loadingMore} onclick={more}>Load older</Button>
		{/if}
	{/if}
</Section>
