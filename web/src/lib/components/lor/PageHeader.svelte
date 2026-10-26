<script lang="ts">
	import type { Snippet } from 'svelte';
	let { title, subtitle, actions }: { title: string; subtitle?: string; actions?: Snippet } =
		$props();
</script>

<header class="mb-6 flex flex-wrap items-end justify-between gap-3">
	<div class="min-w-0">
		<h1 class="text-2xl">{title}</h1>
		{#if subtitle}<p class="mt-1 text-muted-foreground">{subtitle}</p>{/if}
	</div>
	{#if actions}<div class="flex gap-2">{@render actions()}</div>{/if}
</header>
