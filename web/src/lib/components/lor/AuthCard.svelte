<script lang="ts">
	// The signed-out screens (login, first-time setup): mark, title, one task.
	import type { Snippet } from 'svelte';
	import LorMark from './LorMark.svelte';

	let {
		title,
		subtitle,
		children,
		footer
	}: { title: string; subtitle?: Snippet; children: Snippet; footer?: Snippet } = $props();
</script>

<svelte:head><title>{title} · Lor</title></svelte:head>

<div class="mx-auto flex min-h-[calc(100dvh-6rem)] max-w-sm flex-col justify-center py-10">
	<div class="flex flex-col items-center text-center">
		<LorMark class="mb-5" />
		<h1 class="text-title">{title}</h1>
		{#if subtitle}<p class="mt-2 text-callout text-pretty text-muted-foreground">
				{@render subtitle()}
			</p>{/if}
	</div>
	<div class="mt-8">{@render children()}</div>
	{#if footer}<div class="mt-8 text-center text-footnote text-muted-foreground">
			{@render footer()}
		</div>{/if}
</div>
