<script lang="ts" generics="T extends string | number">
	// Mutually exclusive view switch (2–5 short options). A radio group:
	// arrow keys move the selection, only the selected segment is tabbable.
	import type { Component } from 'svelte';
	import { segmentedVariants } from '#lib/design/variants.js';
	import { onRadioKeydown } from '#lib/design/roving.js';
	import { cn } from '#lib/utils.js';

	let {
		options,
		value = $bindable(),
		label,
		fill = false,
		onchange,
		class: className
	}: {
		options: { value: T; label: string; icon?: Component }[];
		value: T;
		/** Accessible name of the group (not shown). */
		label: string;
		/** Stretch segments to the container width. */
		fill?: boolean;
		onchange?: (value: T) => void;
		class?: string;
	} = $props();

	const s = $derived(segmentedVariants({ fill }));
	function select(i: number) {
		value = options[i].value;
		onchange?.(value);
	}
</script>

<div
	role="radiogroup"
	aria-label={label}
	data-slot="segmented-control"
	tabindex="-1"
	class={cn(s.root(), className)}
	onkeydown={(e) => onRadioKeydown(e, select)}
>
	{#each options as o, i (o.value)}
		<button
			type="button"
			role="radio"
			aria-checked={o.value === value}
			tabindex={o.value === value ? 0 : -1}
			data-index={i}
			class={s.item()}
			onclick={() => select(i)}
		>
			{#if o.icon}<o.icon aria-hidden="true" />{/if}{o.label}
		</button>
	{/each}
</div>
