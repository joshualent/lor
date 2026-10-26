<script lang="ts">
	import '../app.css';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import { Toaster } from '#lib/components/ui/sonner/index.js';
	import Shell from '#lib/components/lor/Shell.svelte';
	import { ApiError } from '#lib/api/error.js';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const queryClient = new QueryClient({
		defaultOptions: {
			queries: {
				refetchOnWindowFocus: true,
				retry: (count, err) =>
					!(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 3
			}
		}
	});
</script>

<svelte:head>
	<title>Lor</title>
</svelte:head>

<QueryClientProvider client={queryClient}>
	<Shell>
		{@render children()}
	</Shell>
	<Toaster />
</QueryClientProvider>
