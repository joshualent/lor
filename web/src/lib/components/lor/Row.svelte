<script lang="ts">
	import type { Snippet } from 'svelte';
	let { label, children }: { label: string; children: Snippet } = $props();
</script>

<div class="grid grid-cols-[7.5rem_1fr] gap-3 py-1.5 sm:grid-cols-[9rem_1fr]">
	<dt class="text-muted-foreground">{label}</dt>
	<dd class="min-w-0 break-words">{@render children()}</dd>
</div>
