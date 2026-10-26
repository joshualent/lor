<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { keys, fetchSession } from '#lib/queries.js';
	import { api, call } from '#lib/api/client.js';

	let { children }: { children: Snippet } = $props();
	const qc = useQueryClient();

	const PUBLIC = ['/login', '/setup'];
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

<div class="mx-auto flex min-h-dvh max-w-3xl flex-col px-4 sm:px-6">
	{#if !isPublic && session.data?.authenticated}
		<header class="flex items-center gap-4 pt-4 pb-2 sm:pt-6">
			<a href="/" class="text-lg font-semibold tracking-tight no-underline!" aria-label="Lor home"
				>Lor</a
			>
			<nav class="-mx-1 no-scrollbar flex flex-1 gap-1 overflow-x-auto" aria-label="Main">
				{#each nav as n (n.href)}
					<a
						href={n.href}
						class="rounded-md px-2.5 py-1.5 text-sm whitespace-nowrap no-underline! {active(n.href)
							? 'bg-secondary text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
						aria-current={active(n.href) ? 'page' : undefined}>{n.label}</a
					>
				{/each}
			</nav>
			<button
				class="hidden text-sm text-muted-foreground hover:text-foreground sm:block"
				onclick={signOut}
			>
				Sign out
			</button>
		</header>
	{/if}
	<main class="flex-1 pt-4 pb-16">
		{#if ready}
			{@render children()}
		{:else if session.isError}
			<p class="mt-16 text-center text-destructive">Can't reach Lor. Retrying…</p>
		{/if}
	</main>
</div>
