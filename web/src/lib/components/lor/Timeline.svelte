<script lang="ts">
	// A grant's history: what happened, who did it, when. Oldest first, with
	// a connecting line; each step's dot carries its outcome tone.
	import type { AuditEvent } from '#lib/queries.js';
	import { auditLook, parseActor } from '#lib/audit.js';
	import { formatTimestamp } from '#lib/format.js';
	import { iconTileVariants } from '#lib/design/variants.js';

	let { events }: { events: AuditEvent[] } = $props();
</script>

<ol class="rounded-xl bg-card px-4 py-1" data-slot="timeline">
	{#each events as ev, i (ev.id)}
		{@const look = auditLook(ev.action)}
		<li class="relative flex gap-3 py-2.5">
			{#if i < events.length - 1}
				<span class="absolute top-10 bottom-0 left-3.5 w-px bg-separator" aria-hidden="true"></span>
			{/if}
			<span class={iconTileVariants({ tone: look.tone, size: 'sm' })} aria-hidden="true">
				<look.icon />
			</span>
			<div class="min-w-0 flex-1 pt-0.5">
				<p class="text-callout font-medium text-foreground">{look.label}</p>
				<p class="text-footnote text-muted-foreground">
					{parseActor(ev.actor).label} ·
					<time class="nums" datetime={ev.ts}>{formatTimestamp(Date.parse(ev.ts))}</time>
				</p>
			</div>
		</li>
	{/each}
</ol>
