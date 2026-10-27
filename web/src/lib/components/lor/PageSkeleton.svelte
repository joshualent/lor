<script lang="ts">
	// Loading placeholder for a whole page: header, then grouped lists.
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import ListSkeleton from './ListSkeleton.svelte';

	let { sections = 2, header = true }: { sections?: number; header?: boolean } = $props();
</script>

<div class="space-y-8" data-slot="page-skeleton">
	{#if header}
		<div class="space-y-2.5" aria-hidden="true">
			<Skeleton class="h-7 w-48" />
			<Skeleton class="h-3.5 w-64 opacity-70" />
		</div>
	{/if}
	{#each Array.from({ length: sections }, (_, i) => i) as i (i)}
		<div class="space-y-2.5">
			<Skeleton class="ml-1 h-3.5 w-28" />
			<ListSkeleton rows={i === 0 ? 2 : 3} />
		</div>
	{/each}
</div>
