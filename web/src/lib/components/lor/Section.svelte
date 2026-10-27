<script lang="ts">
	// A page region: header (title, count, action), content (usually a List
	// or Card) and an optional footnote. Sections are separated by rhythm,
	// not borders.
	import type { Snippet } from 'svelte';
	import SectionHeader from './SectionHeader.svelte';

	let {
		id: anchor,
		title,
		count,
		description,
		footnote,
		action,
		footer,
		children,
		class: className = '',
		...rest
	}: {
		/** Anchor id, so the section can be linked to (#id). */
		id?: string;
		title?: string;
		count?: number;
		description?: string;
		/** Short explanatory text under the content. */
		footnote?: string;
		action?: Snippet;
		/** Custom footer under the content (instead of `footnote`). */
		footer?: Snippet;
		children: Snippet;
		class?: string;
		[key: `data-${string}`]: unknown;
	} = $props();

	const id = `section-${Math.random().toString(36).slice(2, 9)}`;
</script>

<section
	id={anchor}
	class="scroll-mt-20 {className}"
	aria-labelledby={title ? id : undefined}
	data-slot="section"
	{...rest}
>
	{#if title}
		<SectionHeader {title} {id} {count} {description} {action} />
	{/if}
	{@render children()}
	{#if footer}
		<div class="mt-2 px-1">{@render footer()}</div>
	{:else if footnote}
		<p class="mt-2 px-1 text-footnote text-muted-foreground">{footnote}</p>
	{/if}
</section>
