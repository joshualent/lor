<script lang="ts">
	// Loading placeholder for a whole page: header, then grouped lists.
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import ListSkeleton from './ListSkeleton.svelte';

	let {
		sections = 2,
		header = true,
		layout = 'default'
	}: { sections?: number; header?: boolean; layout?: 'default' | 'dashboard' } = $props();
</script>

{#if layout === 'dashboard'}
	<div aria-busy="true" data-slot="dashboard-skeleton">
		<span role="status" class="sr-only">Loading dashboard…</span>
		<div class="mb-8 grid grid-cols-3 gap-2 sm:gap-4" aria-hidden="true">
			{#each [1, 2, 3] as n (n)}<div class="rounded-xl border border-border bg-card p-3.5 sm:p-5">
					<Skeleton class="h-9 w-full sm:h-4 sm:w-2/3" /><Skeleton class="mt-2 h-9 w-10" /><Skeleton
						class="mt-2 hidden h-3 w-3/4 sm:block"
					/>
				</div>{/each}
		</div>
		<div class="grid gap-8 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]">
			{#each [4, 3] as rows (rows)}<div class="space-y-3">
					<Skeleton class="h-4 w-32" /><Skeleton class="h-3 w-3/4" /><ListSkeleton {rows} />
				</div>{/each}
		</div>
	</div>
{:else}
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
{/if}
