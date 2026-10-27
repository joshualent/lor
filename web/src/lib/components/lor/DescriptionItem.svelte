<script lang="ts">
	// One label/value pair. "inline" puts the label on the left and the value
	// on the right (short values); "stacked" puts the label above (long
	// values like fingerprints or notes).
	import type { Snippet } from 'svelte';
	import { cn } from '#lib/utils.js';

	let {
		label,
		layout = 'inline',
		mono = false,
		children,
		action
	}: {
		label: string;
		layout?: 'inline' | 'stacked';
		/** Monospace value (addresses, fingerprints, commands). */
		mono?: boolean;
		children: Snippet;
		/** Optional control at the end of the row. */
		action?: Snippet;
	} = $props();
</script>

<div
	data-slot="description-item"
	class={cn(
		'flex min-h-12 gap-x-4 gap-y-1 px-4 py-3',
		layout === 'inline' ? 'items-baseline justify-between' : 'flex-col'
	)}
>
	<dt class="shrink-0 text-callout text-muted-foreground">{label}</dt>
	<dd
		class={cn(
			'min-w-0 break-words text-foreground',
			layout === 'inline' ? 'text-right' : '',
			mono ? 'font-mono text-callout wrap-anywhere' : 'text-body'
		)}
	>
		{@render children()}
	</dd>
	{#if action}<div class="shrink-0 self-center">{@render action()}</div>{/if}
</div>
