<script lang="ts">
	// One audit event: outcome-toned category icon, what happened, who did
	// it and when. Tapping it discloses the recorded details and a link to
	// the grant.
	import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
	import type { AuditEvent } from '#lib/queries.js';
	import type { Names } from '#lib/names.svelte.js';
	import { auditLook, detailEntries, parseActor } from '#lib/audit.js';
	import { clock } from '#lib/clock.svelte.js';
	import { formatAgo, formatTimeOfDay, formatTimestamp } from '#lib/format.js';
	import IconTile from './IconTile.svelte';
	import ListRow from './ListRow.svelte';

	let {
		event: ev,
		time = 'clock',
		names
	}: {
		event: AuditEvent;
		/** Host and agent names by id, to say what the event was about. */
		names?: Names;
		/** clock: time of day (rows grouped by day); ago: "2 min ago". */
		time?: 'clock' | 'ago';
	} = $props();

	const look = $derived(auditLook(ev.action));
	const actor = $derived(parseActor(ev.actor));
	const details = $derived(detailEntries(ev.details));
	// "You (iPhone) · nas · grant #7": who, about what.
	const subtitle = $derived.by(() => {
		const host = ev.host_id ? names?.hosts[ev.host_id] : undefined;
		const agent = ev.agent_id ? names?.agents[ev.agent_id] : undefined;
		return [
			actor.label,
			agent && agent !== actor.name ? agent : undefined,
			host,
			ev.grant_id ? `grant #${ev.grant_id}` : undefined
		]
			.filter(Boolean)
			.join(' · ');
	});
	const ms = $derived(Date.parse(ev.ts));
	const expandable = $derived(details.length > 0 || !!ev.grant_id);
	let open = $state(false);
	const regionId = $derived(`audit-${ev.id}`);
</script>

<ListRow
	onclick={expandable ? () => (open = !open) : undefined}
	expanded={expandable ? open : undefined}
	controls={expandable ? regionId : undefined}
	data-action={ev.action}
	density="compact"
>
	{#snippet leading()}<IconTile icon={look.icon} tone={look.tone} size="sm" />{/snippet}
	{#snippet titleContent()}
		<span class="text-callout font-medium">{look.label}</span>
	{/snippet}
	{#snippet subtitleContent()}<span class="text-footnote">{subtitle}</span>{/snippet}
	{#snippet metaContent()}
		<time datetime={ev.ts} title={formatTimestamp(ms)}
			>{time === 'ago' ? formatAgo(ms, clock.now) : formatTimeOfDay(ms)}</time
		>
	{/snippet}
	{#if expandable && open}
		<div id={regionId} class="px-4 pb-3 pl-14">
			<div class="space-y-1.5 rounded-lg bg-muted px-3 py-2.5">
				<p class="font-mono text-caption text-muted-foreground">{ev.action}</p>
				{#if details.length}
					<dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-footnote">
						{#each details as [k, v] (k)}
							<dt class="text-muted-foreground">{k}</dt>
							<dd class="min-w-0 font-mono wrap-anywhere text-foreground">{v}</dd>
						{/each}
					</dl>
				{/if}
				{#if ev.grant_id}
					<a
						href="/grants/{ev.grant_id}"
						class="inline-flex items-center gap-1 pt-1 text-footnote font-medium text-primary"
						>Open grant #{ev.grant_id}<ArrowUpRight class="size-3.5" aria-hidden="true" /></a
					>
				{/if}
			</div>
		</div>
	{/if}
</ListRow>
