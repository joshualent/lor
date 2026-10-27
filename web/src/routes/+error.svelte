<script lang="ts">
	import SearchX from '@lucide/svelte/icons/search-x';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import { page } from '$app/state';
	import { Button } from '#lib/components/ui/button/index.js';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';

	const notFound = $derived(page.status === 404);
</script>

<svelte:head><title>{notFound ? 'Not found' : 'Error'} · Lor</title></svelte:head>

<EmptyState
	icon={notFound ? SearchX : TriangleAlert}
	tone={notFound ? 'neutral' : 'danger'}
	variant="page"
	title={notFound ? 'Page not found' : 'Something went wrong'}
	description={notFound
		? "There's nothing at this address. Approval links look like /r/42 or /e/7."
		: (page.error?.message ?? 'An unexpected error happened.')}
	role="alert"
>
	{#snippet action()}<Button variant="secondary" href="/">Go home</Button>{/snippet}
</EmptyState>
