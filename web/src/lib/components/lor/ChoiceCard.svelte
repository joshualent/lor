<script lang="ts" generics="T extends string">
	// A large radio option with a title and explanation (access level). Uses a
	// real radio input, so forms and keyboard navigation work natively.
	import type { Snippet } from 'svelte';
	import { choiceCardVariants, toneText, type Tone } from '#lib/design/variants.js';

	let {
		name,
		value,
		group = $bindable(),
		title,
		tone,
		children,
		badge
	}: {
		name: string;
		value: T;
		group: T;
		title: string;
		/** Colors the title when selected (e.g. warning for sudo). */
		tone?: Tone;
		/** Explanation under the title. */
		children?: Snippet;
		badge?: Snippet;
	} = $props();

	const selected = $derived(group === value);
	const s = $derived(choiceCardVariants({ selected }));
</script>

<label class={s.root()} data-slot="choice-card" data-selected={selected}>
	<input type="radio" {name} {value} bind:group class="peer sr-only" />
	<span class={s.indicator()} aria-hidden="true"><span class={s.dot()}></span></span>
	<span class="min-w-0 flex-1">
		<span class="flex flex-wrap items-center gap-2">
			<span class="font-medium {selected && tone ? toneText[tone] : ''}">{title}</span>
			{#if badge}{@render badge()}{/if}
		</span>
		{#if children}<span class="mt-0.5 block text-callout text-muted-foreground"
				>{@render children()}</span
			>{/if}
	</span>
</label>
