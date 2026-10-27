<script lang="ts">
	// A form field: label, control, and a hint or error underneath.
	import type { Snippet } from 'svelte';
	import { Label } from '#lib/components/ui/label/index.js';

	let {
		label,
		for: forId,
		hint,
		error,
		optional = false,
		children
	}: {
		label: string;
		for: string;
		hint?: string;
		error?: string | null;
		optional?: boolean;
		children: Snippet;
	} = $props();
</script>

<div class="space-y-2" data-slot="field">
	<Label for={forId}
		>{optional ? `${label} ` : label}{#if optional}<span class="font-normal text-muted-foreground"
				>(optional)</span
			>{/if}</Label
	>
	{@render children()}
	{#if error}
		<p class="text-footnote text-destructive" id="{forId}-error" role="alert">{error}</p>
	{:else if hint}
		<p class="text-footnote text-muted-foreground" id="{forId}-hint">{hint}</p>
	{/if}
</div>
