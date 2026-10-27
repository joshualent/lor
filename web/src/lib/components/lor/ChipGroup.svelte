<script lang="ts" generics="T extends string | number">
	// A wrap-around set of choice chips (durations, categories). A radio
	// group with roving focus. `scroll` keeps one line and scrolls
	// horizontally (filters on phones).
	import type { Snippet } from 'svelte';
	import { chipVariants } from '#lib/design/variants.js';
	import { onRadioKeydown } from '#lib/design/roving.js';
	import { cn } from '#lib/utils.js';

	let {
		options,
		value = $bindable(),
		label,
		labelledby,
		scroll = false,
		onchange,
		after,
		extraSelected = false,
		class: className
	}: {
		options: { value: T; label: string; selected?: boolean }[];
		value: T | null;
		label?: string;
		labelledby?: string;
		scroll?: boolean;
		onchange?: (value: T) => void;
		/**
		 * Extra radio chips after the options (e.g. "Custom"), with
		 * data-index continuing after the options; they manage their own state
		 * (arrow keys "click" them).
		 */
		after?: Snippet<[{ chipClass: (selected: boolean) => string }]>;
		/** An extra chip is the selected one (so no option takes the tab stop). */
		extraSelected?: boolean;
		class?: string;
	} = $props();

	// An option can force its selected state (e.g. a preset that's not active
	// while a custom value is being typed).
	const isSelected = (o: { value: T; selected?: boolean }) => o.selected ?? o.value === value;
	const anySelected = $derived(options.some(isSelected));

	function select(i: number) {
		if (i >= options.length) {
			el?.querySelector<HTMLElement>(`[data-index="${i}"]`)?.click();
			return;
		}
		value = options[i].value;
		onchange?.(value);
	}
	const chipClass = (selected: boolean) => chipVariants({ selected });

	let el = $state<HTMLElement | null>(null);
	$effect(() => {
		void value;
		if (!scroll || !el) return;
		const chip = el.querySelector<HTMLElement>('[aria-checked="true"]');
		if (!chip) return;
		const left = chip.offsetLeft - el.offsetLeft;
		if (left < el.scrollLeft || left + chip.offsetWidth > el.scrollLeft + el.clientWidth) {
			el.scrollTo({ left: left - 16, behavior: 'instant' });
		}
	});
</script>

<div
	bind:this={el}
	role="radiogroup"
	aria-label={label}
	aria-labelledby={labelledby}
	data-slot="chip-group"
	tabindex="-1"
	class={cn(
		'flex gap-2',
		scroll
			? '-mx-4 -my-1.5 no-scrollbar overflow-x-auto px-4 py-1.5 sm:mx-0 sm:flex-wrap sm:px-0'
			: 'flex-wrap',
		className
	)}
	onkeydown={(e) => onRadioKeydown(e, select)}
>
	{#each options as o, i (o.value)}
		<button
			type="button"
			role="radio"
			aria-checked={isSelected(o)}
			tabindex={isSelected(o) || (!anySelected && !extraSelected && i === 0) ? 0 : -1}
			data-index={i}
			class={chipClass(isSelected(o))}
			onclick={() => select(i)}>{o.label}</button
		>
	{/each}
	{@render after?.({ chipClass })}
</div>
