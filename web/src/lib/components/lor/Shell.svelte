<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { dev } from '$app/env';
	import { keys, fetchDashboard, fetchSession } from '#lib/queries.js';
	import { api, call } from '#lib/api/client.js';
	import EmptyState from './EmptyState.svelte';
	import LorMark from './LorMark.svelte';
	import House from '@lucide/svelte/icons/house';
	import Server from '@lucide/svelte/icons/server';
	import Bot from '@lucide/svelte/icons/bot';
	import ScrollText from '@lucide/svelte/icons/scroll-text';
	import Settings2 from '@lucide/svelte/icons/settings-2';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/errors.js';
	import LogOut from '@lucide/svelte/icons/log-out';
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
		{ href: '/', label: 'Home', icon: House },
		{ href: '/hosts', label: 'Hosts', icon: Server },
		{ href: '/agents', label: 'Agents', icon: Bot },
		{ href: '/audit', label: 'Audit', icon: ScrollText },
		{ href: '/settings', label: 'Settings', icon: Settings2 }
	];
	function active(href: string) {
		const p = page.url.pathname;
		if (href === '/')
			return p === '/' || p.startsWith('/r/') || p.startsWith('/e/') || p.startsWith('/grants');
		return p.startsWith(href);
	}

	let signingOut = $state(false);
	async function signOut() {
		if (signingOut) return;
		signingOut = true;
		try {
			await call(api.POST('/logout', { body: {} }));
			qc.clear();
			await goto('/login', { replaceState: true });
		} catch (e) {
			toast.error(`Couldn't sign out: ${errorMessage(e)}`);
		} finally {
			signingOut = false;
		}
	}

	const ready = $derived(isPublic || session.data?.authenticated === true);
</script>

<div class="min-h-dvh">
	<a
		href="#main-content"
		class="fixed top-3 left-3 z-50 -translate-y-24 rounded-md bg-primary px-4 py-3 text-primary-foreground focus:translate-y-0"
		>Skip to content</a
	>
	{#if !isPublic && session.data?.authenticated}
		<header
			class="sticky top-0 z-30 flex items-center gap-1 border-b border-border bg-background/95 px-2 py-2 pt-[max(0.5rem,env(safe-area-inset-top))] backdrop-blur-md lg:fixed lg:inset-y-0 lg:left-0 lg:flex lg:w-52 lg:flex-col lg:gap-0 lg:border-r lg:border-b-0 lg:px-0 lg:py-0"
			data-slot="app-navigation"
		>
			<div class="hidden items-center justify-between lg:flex lg:px-5 lg:pt-7 lg:pb-8">
				<a
					href="/"
					class="flex min-h-11 items-center gap-2.5 rounded-md no-underline! focus-ring"
					aria-label="Lor home"
				>
					<LorMark class="size-9" />
					<span class="text-headline font-semibold tracking-tight"
						>Lor<span
							class="ml-2 text-footnote font-normal tracking-normal text-muted-foreground lg:mt-0.5 lg:ml-0 lg:block"
							>Access control</span
						></span
					>
				</a>
			</div>
			<nav
				class="grid w-full grid-cols-5 gap-1 lg:flex lg:flex-col lg:gap-1.5 lg:px-3"
				aria-label="Main"
			>
				{#each nav as n (n.href)}
					<a
						href={n.href}
						class="relative flex min-h-12 flex-col items-center justify-center gap-1 rounded-md px-1 py-2 text-caption font-medium no-underline! focus-ring transition-colors lg:min-h-11 lg:flex-row lg:justify-start lg:gap-3 lg:px-3 lg:text-callout {active(
							n.href
						)
							? 'bg-secondary text-foreground'
							: 'text-muted-foreground hover:bg-card hover:text-foreground'}"
						aria-current={active(n.href) ? 'page' : undefined}
					>
						<n.icon class="size-[1.125rem] shrink-0" aria-hidden="true" />
						<span>{n.label}</span>
						{#if n.href === '/' && pending > 0}<span
								class="absolute top-1 right-1 grid min-w-4 place-items-center rounded-full bg-warning px-1 text-caption leading-4 font-semibold text-primary-foreground nums lg:static lg:ml-auto"
								><span aria-hidden="true">{pending}</span><span class="sr-only"
									>{pending} waiting for review</span
								></span
							>{/if}
					</a>
				{/each}
			</nav>
			<div class="mt-auto hidden space-y-4 p-4 lg:block">
				<p
					class="border-t border-border px-2 pt-4 text-footnote leading-relaxed text-muted-foreground"
				>
					Temporary access.<br />Always approved by you.
				</p>
				<Button
					variant="ghost"
					disabled={signingOut}
					onclick={signOut}
					class="w-full justify-start text-muted-foreground"
					>{#if signingOut}<Spinner />{:else}<LogOut />{/if}
					{signingOut ? 'Signing out…' : 'Sign out'}</Button
				>
			</div>
		</header>
	{/if}
	<div class={!isPublic && session.data?.authenticated ? 'lg:pl-52' : ''}>
		<main
			id="main-content"
			tabindex="-1"
			class="mx-auto w-full flex-1 px-4 pt-6 pb-12 outline-none sm:px-6 sm:pt-8 lg:px-8 lg:pt-10 {isPublic
				? 'max-w-3xl'
				: page.url.pathname === '/' ||
					  ['/hosts', '/agents', '/audit', '/settings'].includes(page.url.pathname)
					? 'max-w-7xl'
					: 'max-w-3xl'}"
		>
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
</div>
