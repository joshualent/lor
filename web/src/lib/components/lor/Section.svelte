<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		title,
		children,
		actions,
		class: className = ''
	}: { title?: string; children: Snippet; actions?: Snippet; class?: string } = $props();
</script>

<section class="rounded-xl bg-card p-4 sm:p-5 {className}">
	{#if title || actions}
		<div class="mb-3 flex items-center justify-between gap-3">
			{#if title}<h2 class="text-base">{title}</h2>{/if}
			{#if actions}{@render actions()}{/if}
		</div>
	{/if}
	{@render children()}
</section>
