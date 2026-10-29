<script lang="ts">
	// One row of a List. Renders a link (href), a button (onclick) or static
	// content, with slots for a leading tile, title/subtitle, trailing
	// metadata and separate trailing controls. Links get a chevron.
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import { listRowVariants } from '#lib/design/variants.js';
	import { cn } from '#lib/utils.js';

	let {
		href,
		onclick,
		title,
		subtitle,
		meta,
		chevron,
		reserveChevron = false,
		density = 'default',
		stackMeta = false,
		compactOnNarrow = false,
		tone = 'default',
		disabled = false,
		expanded,
		controls,
		label,
		leading,
		titleContent,
		subtitleContent,
		metaContent,
		trailing,
		children,
		class: className,
		...rest
	}: {
		href?: string;
		onclick?: (e: MouseEvent) => void;
		title?: string;
		subtitle?: string;
		/** Short trailing text: a time, a count, a value. */
		meta?: string;
		/** Defaults to true for links. */
		chevron?: boolean;
		/** Keep static rows aligned with disclosure rows. */
		reserveChevron?: boolean;
		density?: 'default' | 'compact' | 'relaxed';
		/** Put metadata below the text on phones, for long addresses or summaries. */
		stackMeta?: boolean;
		/** Hide decorative leading tiles and tighten layout below 400px. */
		compactOnNarrow?: boolean;
		tone?: 'default' | 'danger' | 'accent';
		disabled?: boolean;
		/** For disclosure rows (button): aria-expanded. */
		expanded?: boolean;
		/** For disclosure rows: id of the region it controls. */
		controls?: string;
		/** Accessible name for the link/button when the visible text isn't enough. */
		label?: string;
		leading?: Snippet;
		titleContent?: Snippet;
		subtitleContent?: Snippet;
		metaContent?: Snippet;
		/** Separate controls (buttons), outside the row's link/button. */
		trailing?: Snippet;
		/** Extra content below the row (e.g. expanded details). */
		children?: Snippet;
		class?: string;
	} & Omit<HTMLAttributes<HTMLLIElement>, 'title' | 'onclick'> = $props();

	const interactive = $derived(!!href || !!onclick);
	const showChevron = $derived(chevron ?? !!href);
	const s = $derived(
		listRowVariants({
			interactive,
			hasTrailing: !!trailing,
			density,
			tone,
			disabled,
			stackMeta,
			compactOnNarrow
		})
	);
</script>

{#snippet body()}
	{#if leading}<span class={s.leading()}>{@render leading()}</span>{/if}
	<span class={s.content()}>
		{#if titleContent}
			<span class={s.title()}>{@render titleContent()}</span>
		{:else if title}
			<span class={s.title()}>{title}</span>
		{/if}
		{#if subtitleContent}
			<span class={s.subtitle()}>{@render subtitleContent()}</span>
		{:else if subtitle}
			<span class={s.subtitle()}>{subtitle}</span>
		{/if}
	</span>
	{#if metaContent}
		<span class={s.meta()}>{@render metaContent()}</span>
	{:else if meta}
		<span class={s.meta()}>{meta}</span>
	{/if}
	{#if expanded !== undefined}
		<ChevronDown
			class="{s.chevron()} {expanded ? 'rotate-180' : ''}"
			aria-hidden="true"
			data-slot="disclosure"
		/>
	{:else if showChevron}<ChevronRight
			class={s.chevron()}
			aria-hidden="true"
		/>{:else if reserveChevron}<span class={s.chevron()} aria-hidden="true"></span>{/if}
{/snippet}

<li class={cn(s.root(), expanded && 'bg-card-hover/60', className)} data-slot="list-row" {...rest}>
	<div class="flex min-w-0 flex-1 flex-col">
		{#if href && !disabled}
			<a {href} class={s.main()} aria-label={label}>{@render body()}</a>
		{:else if onclick}
			<button
				type="button"
				class={s.main()}
				{onclick}
				{disabled}
				aria-label={label}
				aria-expanded={expanded}
				aria-controls={controls}>{@render body()}</button
			>
		{:else}
			<div class={s.main()}>{@render body()}</div>
		{/if}
		{#if children}{@render children()}{/if}
	</div>
	{#if trailing}<div class={s.trailing()}>{@render trailing()}</div>{/if}
</li>
