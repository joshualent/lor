<script lang="ts">
	// A failed load: the server's message (as is) and a retry.
	import CloudOff from '@lucide/svelte/icons/cloud-off';
	import SearchX from '@lucide/svelte/icons/search-x';
	import RotateCw from '@lucide/svelte/icons/rotate-cw';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import { ApiError } from '#lib/api/error.js';
	import { errorMessage } from '#lib/errors.js';
	import EmptyState from './EmptyState.svelte';

	let {
		error,
		title,
		onretry,
		back,
		fetching = false,
		variant = 'page'
	}: {
		error: unknown;
		title?: string;
		onretry?: () => void;
		/** Where to go instead, e.g. { href: '/hosts', label: 'All hosts' }. */
		back?: { href: string; label: string };
		variant?: 'inset' | 'page';
		/** A retry is in flight. */
		fetching?: boolean;
	} = $props();

	const notFound = $derived(error instanceof ApiError && error.status === 404);
</script>

<EmptyState
	icon={notFound ? SearchX : CloudOff}
	tone={notFound ? 'neutral' : 'danger'}
	title={title ?? (notFound ? 'Not found' : "Couldn't load this")}
	description={errorMessage(error)}
	{variant}
	role="alert"
>
	{#if notFound}<p class="mt-3 max-w-sm text-callout text-muted-foreground">
			Check that the link is complete, or use the button below to return.
		</p>{/if}
	{#snippet action()}
		{#if onretry && !notFound}
			<Button variant="secondary" disabled={fetching} onclick={onretry}
				>{#if fetching}<Spinner />{:else}<RotateCw />{/if} Try again</Button
			>
		{/if}
		{#if back}<Button variant={notFound ? 'secondary' : 'ghost'} href={back.href}
				>{back.label}</Button
			>{/if}
	{/snippet}
</EmptyState>
