<script lang="ts">
	// A linked page summary. Counts are data, and color only marks a state.
	import type { Component } from 'svelte';
	import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
	import { summaryCardVariants, toneText, type Tone } from '#lib/design/variants.js';
	let {
		href,
		label,
		value,
		description,
		icon: Icon,
		tone = 'neutral'
	}: {
		href: string;
		label: string;
		value: number;
		description: string;
		icon: Component<{ class?: string; 'aria-hidden'?: 'true' | 'false' | boolean }>;
		tone?: Tone;
	} = $props();
	const styles = summaryCardVariants();
</script>

<a {href} class={styles.root()} data-slot="summary-card">
	<span class={styles.label()}
		><span class="flex items-center gap-2"
			><Icon class="hidden size-4 sm:block" aria-hidden="true" />{label}</span
		><ArrowUpRight class="hidden size-3.5 text-icon-muted sm:block" aria-hidden="true" /></span
	>
	<span class={styles.value({ class: value > 0 ? toneText[tone] : 'text-foreground' })}
		>{value}</span
	>
	<span class={styles.description()}>{description}</span>
</a>
