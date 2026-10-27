<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { dev } from '$app/env';
	import { keys, fetchDashboard, fetchSession } from '#lib/queries.js';
	import { api, call } from '#lib/api/client.js';
	import EmptyState from './EmptyState.svelte';
	import CloudOff from '@lucide/svelte/icons/cloud-off';
	import RotateCw from '@lucide/svelte/icons/rotate-cw';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';

	let { children }: { children: Snippet } = $props();
	const qc = useQueryClient();

	// The component gallery needs no data; it only exists in dev builds.
	const PUBLIC = dev ? ['/login', '/setup', '/gallery'] : ['/login', '/setup'];
	const isPublic = $derived(PUBLIC.includes(page.url.pathname));

	const session = createQuery(() => ({
		queryKey: keys.session,
		queryFn: fetchSession,
		staleTime: 30_000
	}));

	// Route guard: signed-out users go to /login?next=<here>; before the first
	// passkey exists everything goes to /setup.
	$effect(() => {
		const s = session.data;
		if (!s) return;
		const path = page.url.pathname;
		if (s.setup_required && path !== '/setup') {
			goto('/setup', { replaceState: true });
		} else if (!s.authenticated && !isPublic) {
			goto(`/login?next=${encodeURIComponent(path + page.url.search)}`, { replaceState: true });
		}
	});

	// Pending decisions, as a badge on Home (shares the dashboard's cache;
	// the dashboard itself polls faster while it's open).
	const dash = createQuery(() => ({
		queryKey: keys.dashboard,
		queryFn: fetchDashboard,
		refetchInterval: 30_000,
		enabled: session.data?.authenticated === true
	}));
	const pending = $derived(
		(dash.data?.pending_requests.length ?? 0) + (dash.data?.pending_extensions.length ?? 0)
	);

	const nav = [
		{ href: '/', label: 'Home' },
		{ href: '/hosts', label: 'Hosts' },
		{ href: '/agents', label: 'Agents' },
		{ href: '/audit', label: 'Audit' },
		{ href: '/settings', label: 'Settings' }
	];
	function active(href: string) {
		const p = page.url.pathname;
		if (href === '/')
			return p === '/' || p.startsWith('/r/') || p.startsWith('/e/') || p.startsWith('/grants');
		return p.startsWith(href);
	}

	async function signOut() {
		await call(api.POST('/logout', { body: {} })).catch(() => {});
		qc.clear();
		await goto('/login', { replaceState: true });
	}

	const ready = $derived(isPublic || session.data?.authenticated === true);
</script>

<div class="flex min-h-dvh flex-col">
	{#if !isPublic && session.data?.authenticated}
		<header
			class="sticky top-0 z-30 border-b border-transparent bg-background/85 backdrop-blur-md supports-[backdrop-filter]:bg-background/70"
		>
			<div
				class="mx-auto flex max-w-3xl items-center gap-3 px-4 pt-[max(0.75rem,env(safe-area-inset-top))] pb-2.5 sm:gap-4 sm:px-6 sm:pt-5"
			>
				<a
					href="/"
					class="rounded-md text-lg font-semibold tracking-tight no-underline! focus-ring"
					aria-label="Lor home">Lor</a
				>
				<nav
					class="-mx-1 no-scrollbar flex flex-1 gap-0.5 overflow-x-auto sm:gap-1"
					aria-label="Main"
				>
					{#each nav as n (n.href)}
						<a
							href={n.href}
							class="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1.5 text-sm whitespace-nowrap no-underline! focus-ring transition-colors sm:px-2.5 {active(
								n.href
							)
								? 'bg-secondary text-foreground'
								: 'text-muted-foreground hover:text-foreground'}"
							aria-current={active(n.href) ? 'page' : undefined}
							>{n.label}{#if n.href === '/' && pending > 0}<span
									class="grid h-4 min-w-4 place-items-center rounded-full bg-warning px-1 text-[0.6875rem] leading-none font-semibold text-primary-foreground nums"
									aria-label="{pending} waiting">{pending}</span
								>{/if}</a
						>
					{/each}
				</nav>
				<button
					class="hidden rounded-md text-sm text-muted-foreground focus-ring transition-colors hover:text-foreground sm:block"
					onclick={signOut}
				>
					Sign out
				</button>
			</div>
		</header>
	{/if}
	<main class="mx-auto w-full max-w-3xl flex-1 px-4 pt-4 pb-16 sm:px-6 sm:pt-6">
		{#if ready}
			{@render children()}
		{:else if session.isError}
			<EmptyState
				icon={CloudOff}
				tone="danger"
				variant="page"
				title="Can't reach Lor"
				description="Check that the server is running and your connection is up."
				role="alert"
			>
				{#snippet action()}
					<Button
						variant="secondary"
						disabled={session.isFetching}
						onclick={() => session.refetch()}
						>{#if session.isFetching}<Spinner />{:else}<RotateCw />{/if} Try again</Button
					>
				{/snippet}
			</EmptyState>
		{:else}
			<div class="flex min-h-[50dvh] items-center justify-center" role="status">
				<p class="flex items-center gap-2 text-callout text-muted-foreground">
					<Spinner class="size-4" /> Checking your session…
				</p>
			</div>
		{/if}
	</main>
</div>
