<script lang="ts">
	// Title above a grouped list or card, with an optional count and action.
	import type { Snippet } from 'svelte';

	let {
		title,
		id,
		count,
		description,
		action
	}: {
		title: string;
		/** Heading id, so the section can be aria-labelledby it. */
		id?: string;
		/** Number of items, shown as a quiet counter. */
		count?: number;
		description?: string;
		/** A small link or button on the right ("All", "Add"). */
		action?: Snippet;
	} = $props();
</script>

<div class="mb-2 flex min-h-8 items-end justify-between gap-3 px-1" data-slot="section-header">
	<div class="min-w-0">
		<h2 {id} class="flex items-center gap-2 text-heading text-foreground">
			{title}
			{#if count !== undefined}
				<span
					class="rounded-full bg-secondary px-1.5 py-px text-caption font-medium text-muted-foreground nums"
					>{count}</span
				>
			{/if}
		</h2>
		{#if description}<p class="mt-0.5 text-footnote text-muted-foreground">{description}</p>{/if}
	</div>
	{#if action}<div class="flex shrink-0 items-center gap-1">{@render action()}</div>{/if}
</div>
