<script lang="ts">
	// Loading placeholder shaped like a grouped list.
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';

	let {
		rows = 3,
		tile = true,
		label = 'Loading…'
	}: { rows?: number; tile?: boolean; label?: string } = $props();
</script>

<div aria-busy="true" data-slot="list-skeleton">
	<span class="sr-only" role="status">{label}</span>
	<div class="divide-y divide-separator overflow-hidden rounded-xl bg-card">
		{#each Array.from({ length: rows }, (_, i) => i) as i (i)}
			<div class="flex min-h-14 items-center gap-3 px-4 py-3">
				{#if tile}<Skeleton shape="tile" />{/if}
				<div class="flex-1 space-y-2">
					<Skeleton class="w-2/5" />
					<Skeleton class="h-3 w-3/5 opacity-70" />
				</div>
				<Skeleton class="h-3 w-10 opacity-70" />
			</div>
		{/each}
	</div>
</div>
