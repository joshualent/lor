<script lang="ts">
	import { parseGrantFilter } from '#lib/grant-filter.js';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { onDestroy, tick } from 'svelte';
	import ScrollText from '@lucide/svelte/icons/scroll-text';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { createQuery, keepPreviousData } from '@tanstack/svelte-query';
	import { clock } from '#lib/clock.svelte.js';
	import { ACTOR_FILTERS, CATEGORY_FILTERS, groupByDay } from '#lib/audit.js';
	import { errorMessage } from '#lib/errors.js';
	import { keys, fetchAudit, type AuditEvent } from '#lib/queries.js';
	import { useNames } from '#lib/names.svelte.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import AuditEventRow from '#lib/components/lor/AuditEventRow.svelte';
	import ChipGroup from '#lib/components/lor/ChipGroup.svelte';
	import EmptyState from '#lib/components/lor/EmptyState.svelte';
	import ErrorState from '#lib/components/lor/ErrorState.svelte';
	import List from '#lib/components/lor/List.svelte';
	import ListSkeleton from '#lib/components/lor/ListSkeleton.svelte';
	import PageHeader from '#lib/components/lor/PageHeader.svelte';
	import SearchField from '#lib/components/lor/SearchField.svelte';
	import Section from '#lib/components/lor/Section.svelte';
	import SegmentedControl from '#lib/components/lor/SegmentedControl.svelte';
	import Toolbar from '#lib/components/lor/Toolbar.svelte';
	import { NativeSelect } from '#lib/components/ui/native-select/index.js';
	import StaleNotice from '#lib/components/lor/StaleNotice.svelte';

	const PAGE = 100;
	const VIEW_PAGE = 25;
	let pageIndex = $state(0);
	let historyActive = $state(false);
	let headSnapshot = $state<AuditEvent[] | null>(null);

	let actor = $state(page.url.searchParams.get('actor') ?? '');
	let action = $state(page.url.searchParams.get('action') ?? '');
	let grant = $state(page.url.searchParams.get('grant_id') ?? '');
	let grantQuery = $state(page.url.searchParams.get('grant_id') ?? '');
	$effect(() => {
		const value = grant;
		const timer = setTimeout(() => {
			grantQuery = value;
		}, 250);
		return () => clearTimeout(timer);
	});
	let older = $state<AuditEvent[]>([]);
	let olderDone = $state(false);
	let loadingMore = $state(false);
	let moreError = $state<unknown>(null);

	const grantFilter = $derived(parseGrantFilter(grantQuery));
	let historyGeneration = 0;
	onDestroy(() => {
		historyGeneration++;
	});
	const filter = $derived({
		actor: actor || undefined,
		action: action.trim() || undefined,
		grant_id: grantFilter.value
	});
	const q = createQuery(() => ({
		queryKey: keys.audit(filter),
		queryFn: () => fetchAudit(filter),
		refetchInterval: loadingMore || historyActive ? false : 15_000,
		refetchOnWindowFocus: !loadingMore && !historyActive,
		refetchOnReconnect: !loadingMore && !historyActive,
		placeholderData: keepPreviousData,
		enabled: !grantFilter.error
	}));
	function resetHistory() {
		historyGeneration++;
		pageIndex = 0;
		historyActive = false;
		headSnapshot = null;
		older = [];
		olderDone = false;
		loadingMore = false;
		moreError = null;
	}
	$effect(() => {
		void filter;
		resetHistory();
	});
	async function focusPages(revision = historyGeneration) {
		await tick();
		if (revision !== historyGeneration) return;
		const target = document.getElementById('audit-page-navigation');
		target?.focus({ preventScroll: true });
		target?.scrollIntoView({ block: 'start' });
	}
	function backToLatest() {
		resetHistory();
		void q.refetch();
		void focusPages();
	}
	function newerPage() {
		moreError = null;
		if (pageIndex > 0) {
			pageIndex--;
			void focusPages();
		}
	}

	// Keep the filters in the URL, so a filtered log can be shared or reloaded.
	$effect(() => {
		const parts = Object.entries(filter)
			.filter(([, v]) => v !== undefined)
			.map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`);
		const search = parts.length ? `?${parts.join('&')}` : '';
		if (search !== window.location.search) {
			goto(`${window.location.pathname}${search}`, { replace: true, shallow: true });
		}
	});

	const names = useNames();

	const allEvents = $derived([
		...(historyActive ? (headSnapshot ?? []) : (q.data ?? [])),
		...older
	]);
	const events = $derived(allEvents.slice(pageIndex * VIEW_PAGE, (pageIndex + 1) * VIEW_PAGE));
	const groups = $derived(groupByDay(events, clock.now));
	const hasMore = $derived(
		!q.isPlaceholderData &&
			!olderDone &&
			(older.length
				? older.length % PAGE === 0
				: ((historyActive ? headSnapshot?.length : q.data?.length) ?? 0) >= PAGE)
	);
	const canGoOlder = $derived((pageIndex + 1) * VIEW_PAGE < allEvents.length || hasMore);
	const pagingBusy = $derived(loadingMore || q.isFetching || q.isPlaceholderData);
	const pageable = $derived(historyActive || canGoOlder || q.isPlaceholderData);
	const eventRange = $derived(
		events.length === 1
			? `Event ${pageIndex * VIEW_PAGE + 1}`
			: `Events ${pageIndex * VIEW_PAGE + 1}–${pageIndex * VIEW_PAGE + events.length}`
	);
	const filtered = $derived(!!(filter.actor || filter.action || grant.trim()));

	// The category chips, plus the current prefix if it came from a link.
	const categories = $derived(
		CATEGORY_FILTERS.some((c) => c.prefix === action)
			? CATEGORY_FILTERS
			: [...CATEGORY_FILTERS, { label: action, prefix: action }]
	);

	async function olderPage() {
		if (pagingBusy || grantFilter.error) return;
		if (!historyActive) headSnapshot = [...(q.data ?? [])];
		historyActive = true;
		if ((pageIndex + 1) * VIEW_PAGE < allEvents.length) {
			moreError = null;
			pageIndex++;
			void focusPages();
			return;
		}
		const last = allEvents.at(-1);
		if (!last || !hasMore) return;
		const revision = historyGeneration;
		const initiatingFilter = { ...filter };
		loadingMore = true;
		moreError = null;
		try {
			const next = await fetchAudit({ ...initiatingFilter, before: last.id });
			if (
				revision !== historyGeneration ||
				JSON.stringify(initiatingFilter) !== JSON.stringify(filter)
			)
				return;
			if (next.length < PAGE) olderDone = true;
			older = [...older, ...next];
			if (next.length) {
				pageIndex++;
			}
			void focusPages(revision);
		} catch (e) {
			if (revision === historyGeneration) moreError = e;
		} finally {
			if (revision === historyGeneration) loadingMore = false;
		}
	}

	function clear() {
		actor = '';
		action = '';
		grant = '';
		grantQuery = '';
	}
</script>

<PageHeader
	title="Audit log"
	subtitle="Everything Lor did and who asked. Append-only, newest first."
>
	{#snippet actions()}{#if filtered}<Button variant="secondary" onclick={clear}
				>Clear filters</Button
			>{/if}{/snippet}
</PageHeader>

<Toolbar id="audit-filters" label="Filters">
	<div class="space-y-2">
		<p class="text-footnote text-muted-foreground">Who</p>
		<SegmentedControl
			label="Who"
			bind:value={actor}
			options={ACTOR_FILTERS.map((f) => ({ value: f.value, label: f.label }))}
			class="w-full sm:w-auto"
			fill
		/>
	</div>
	<div
		class="grid grid-cols-[minmax(0,1fr)_7.5rem] items-start gap-3 sm:grid-cols-[minmax(0,1fr)_9rem]"
	>
		<div class="min-w-0 space-y-2">
			<label for="audit-category" class="block text-footnote text-muted-foreground sm:hidden"
				>Category</label
			>
			<p class="hidden text-footnote text-muted-foreground sm:block">Category</p>
			<NativeSelect id="audit-category" bind:value={action} class="sm:hidden"
				>{#each categories as c (c.prefix)}<option value={c.prefix}
						>{c.prefix ? c.label : 'All categories'}</option
					>{/each}</NativeSelect
			>
			<ChipGroup
				label="Category"
				bind:value={action}
				options={categories.map((c) => ({ value: c.prefix, label: c.label }))}
				class="hidden sm:flex"
			/>
		</div>
		<div class="space-y-2">
			<label for="audit-grant-number" class="block text-footnote text-muted-foreground"
				>Grant number</label
			><SearchField
				bind:value={grant}
				id="audit-grant-number"
				aria-invalid={!!grantFilter.error}
				aria-describedby={grantFilter.error ? 'audit-grant-error' : undefined}
				label="Grant number"
				placeholder="Grant #"
				inputmode="numeric"
			/>
		</div>
	</div>
</Toolbar>
{#if grantFilter.error}<p
		id="audit-grant-error"
		role="alert"
		class="-mt-2 mb-5 text-footnote text-destructive"
	>
		{grantFilter.error}
	</p>{/if}

{#if q.isError && q.data}<StaleNotice
		error={q.error}
		updatedAt={q.dataUpdatedAt}
		fetching={q.isFetching}
		onretry={backToLatest}
	/>{/if}

<div role="status" aria-live="polite">
	{#if q.isPlaceholderData && !grantFilter.error}<p
			class="mb-4 flex items-center gap-2 text-footnote text-muted-foreground"
		>
			<Spinner class="size-3.5" /> Updating results… {allEvents.length
				? 'Previous results are shown below.'
				: 'Loading matching events.'}
		</p>{/if}
</div>

{#snippet pagination(footer = false)}
	<nav
		id={footer ? undefined : 'audit-page-navigation'}
		tabindex="-1"
		aria-label={footer ? 'Audit pages footer' : 'Audit pages'}
		aria-busy={loadingMore}
		class="scroll-mt-24 space-y-1 rounded-lg focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring sm:max-w-md"
	>
		{#if !footer || pageable}<div
				class="flex items-center justify-between gap-1 sm:justify-start sm:gap-3"
			>
				{#if pageable}<Button
						variant="secondary"
						class="min-h-11 gap-1 px-3 text-footnote"
						aria-label="Newer events"
						disabled={pageIndex === 0 || pagingBusy}
						onclick={newerPage}
					>
						<ChevronLeft class="size-3.5" /> Newer
					</Button>{/if}
				<span
					class="min-w-0 text-center text-footnote wrap-anywhere text-muted-foreground nums"
					aria-live={footer ? 'off' : 'polite'}
				>
					{eventRange}{#if !canGoOlder && !q.isPlaceholderData}<span class="sr-only"
							>. End of recorded history.</span
						>{/if}
				</span>
				{#if pageable}<Button
						variant="secondary"
						class="min-h-11 gap-1 px-3 text-footnote"
						aria-label="Older events"
						disabled={!canGoOlder || pagingBusy}
						onclick={olderPage}
					>
						Older {#if loadingMore}<Spinner class="size-3.5" />{:else}<ChevronRight
								class="size-3.5"
							/>{/if}
					</Button>{/if}
			</div>{/if}
		{#if historyActive || footer}
			<div
				class="flex flex-wrap items-center justify-between gap-x-3 text-footnote text-muted-foreground"
			>
				{#if footer}<Button href="#audit-filters" variant="link" class="min-h-11 text-footnote"
						>Back to filters</Button
					>
				{:else}<span>Live updates paused</span>{/if}
				{#if historyActive}<Button
						variant="link"
						class="min-h-11 text-footnote"
						onclick={backToLatest}>Back to latest</Button
					>{/if}
			</div>
		{/if}
		{#if moreError}
			<p class="text-footnote text-destructive" role={footer ? undefined : 'alert'}>
				Couldn't load older events: {errorMessage(moreError)}
				<span class="block">Select Older to try again.</span>
			</p>
		{/if}
		{#if footer && !canGoOlder && !q.isPlaceholderData}
			<p class="text-center text-footnote text-muted-foreground">End of recorded history.</p>
		{/if}
	</nav>
{/snippet}

{#if grantFilter.error}
	<EmptyState
		title="Enter a valid grant number"
		description="Use the number on the grant, with or without #."
		>{#snippet action()}<Button
				variant="secondary"
				onclick={() => {
					grant = '';
					grantQuery = '';
				}}>Clear grant filter</Button
			>{/snippet}</EmptyState
	>
{:else if q.isPending || (q.isPlaceholderData && events.length === 0)}
	<ListSkeleton rows={8} />
{:else if q.isError && !q.data}
	<ErrorState error={q.error} fetching={q.isFetching} onretry={() => q.refetch()} variant="inset" />
{:else if events.length === 0}
	<EmptyState
		icon={ScrollText}
		title={filtered ? 'No matching events' : 'Nothing logged yet'}
		description={filtered
			? 'Try another filter, or clear them to see everything.'
			: 'Requests, approvals, sign-ins and changes are recorded here.'}
	>
		{#snippet action()}
			{#if filtered}<Button variant="secondary" onclick={clear}>Clear filters</Button>{/if}
		{/snippet}
	</EmptyState>
{:else}
	<div class="mb-5">{@render pagination()}</div>
	<div class="space-y-6" data-testid="audit-log" aria-busy={q.isPlaceholderData}>
		{#each groups as group (group.key)}
			<Section title={group.label} count={group.events.length}>
				<List ordered>
					{#each group.events as ev (ev.id)}
						<AuditEventRow event={ev} names={names.current} />
					{/each}
				</List>
			</Section>
		{/each}
		{@render pagination(true)}
	</div>
{/if}
