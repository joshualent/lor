<script lang="ts">
	// The top of every page: optional back link (list → detail), an optional
	// leading tile, the title, a subtitle, badges (`meta`) and the page's
	// actions. Also sets the document title.
	import type { Snippet } from 'svelte';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';

	let {
		title,
		subtitle,
		back,
		leading,
		heading,
		meta,
		actions,
		documentTitle
	}: {
		/** Page title (also the document title unless `documentTitle` is set). */
		title: string;
		subtitle?: string;
		/** Parent page for detail views, e.g. { href: '/hosts', label: 'Hosts' }. */
		back?: { href: string; label: string };
		/** Icon tile or avatar shown before the title. */
		leading?: Snippet;
		/** Custom title markup; `title` is still used for the document title. */
		heading?: Snippet;
		/** Badges or status under the title. */
		meta?: Snippet;
		/** Page actions; the primary one uses the default button. */
		actions?: Snippet;
		documentTitle?: string;
	} = $props();
</script>

<svelte:head>
	<title>{documentTitle ?? title} · Lor</title>
</svelte:head>

<header class="mb-6 sm:mb-8" data-slot="page-header">
	{#if back}
		<a
			href={back.href}
			class="mb-3 -ml-1.5 inline-flex items-center gap-0.5 rounded-md py-1 pr-2 pl-0.5 text-callout text-muted-foreground no-underline! focus-ring transition-colors hover:text-foreground"
		>
			<ChevronLeft class="size-4" aria-hidden="true" />{back.label}
		</a>
	{/if}
	<div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
		<div class="flex min-w-0 items-center gap-3.5">
			{#if leading}{@render leading()}{/if}
			<div class="min-w-0">
				<h1 class="text-title break-words" aria-label={heading ? title : undefined}>
					{#if heading}{@render heading()}{:else}{title}{/if}
				</h1>
				{#if subtitle}<p class="mt-1 text-callout text-muted-foreground">{subtitle}</p>{/if}
			</div>
		</div>
		{#if actions}<div class="flex shrink-0 flex-wrap gap-2">{@render actions()}</div>{/if}
	</div>
	{#if meta}<div class="mt-3 flex flex-wrap items-center gap-2">{@render meta()}</div>{/if}
</header>
