<script lang="ts">
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

	const filter = $derived({
		actor: actor || undefined,
		action: action.trim() || undefined,
		grant_id: Number(grantQuery) || undefined
	});
	const q = createQuery(() => ({
		queryKey: keys.audit(filter),
		queryFn: () => fetchAudit(filter),
		refetchInterval: 15_000,
		placeholderData: keepPreviousData
	}));
	$effect(() => {
		void filter;
		older = [];
		olderDone = false;
	});

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

	const events = $derived([...(q.data ?? []), ...older]);
	const groups = $derived(groupByDay(events, clock.now));
	const hasMore = $derived(
		!q.isPlaceholderData &&
			!olderDone &&
			(older.length ? older.length % PAGE === 0 : (q.data?.length ?? 0) >= PAGE)
	);
	const filtered = $derived(!!(filter.actor || filter.action || filter.grant_id));

	// The category chips, plus the current prefix if it came from a link.
	const categories = $derived(
		CATEGORY_FILTERS.some((c) => c.prefix === action)
			? CATEGORY_FILTERS
			: [...CATEGORY_FILTERS, { label: action, prefix: action }]
	);

	async function more() {
		const last = events.at(-1);
		if (!last) return;
		loadingMore = true;
		moreError = null;
		try {
			const next = await fetchAudit({ ...filter, before: last.id });
			if (next.length < PAGE) olderDone = true;
			older = [...older, ...next];
		} catch (e) {
			moreError = e;
		} finally {
			loadingMore = false;
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

<Toolbar label="Filters">
	<SegmentedControl
		label="Who"
		bind:value={actor}
		options={ACTOR_FILTERS.map((f) => ({ value: f.value, label: f.label }))}
		class="w-full sm:w-auto"
		fill
	/>
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
		<div class="sm:hidden">
			<label for="audit-category" class="mb-1 block text-footnote text-muted-foreground"
				>Category</label
			><NativeSelect id="audit-category" bind:value={action}
				>{#each categories as c (c.prefix)}<option value={c.prefix}
						>{c.prefix ? c.label : 'All categories'}</option
					>{/each}</NativeSelect
			>
		</div>
		<ChipGroup
			label="Category"
			bind:value={action}
			options={categories.map((c) => ({ value: c.prefix, label: c.label }))}
			class="hidden sm:flex sm:flex-1"
		/>
		<SearchField
			bind:value={grant}
			label="Grant number"
			placeholder="Grant #"
			inputmode="numeric"
			class="sm:w-36"
		/>
	</div>
</Toolbar>

{#if q.isError && q.data}<StaleNotice
		error={q.error}
		updatedAt={q.dataUpdatedAt}
		fetching={q.isFetching}
		onretry={() => q.refetch()}
	/>{/if}

{#if q.isPending}
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
	{#if q.isPlaceholderData}<p
			role="status"
			class="mb-4 flex items-center gap-2 text-footnote text-muted-foreground"
		>
			<Spinner class="size-3.5" /> Updating results… Previous results are shown below.
		</p>{/if}
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
		{#if moreError}
			<p class="text-center text-footnote text-destructive" role="alert">
				Couldn't load older events: {errorMessage(moreError)}
			</p>
		{/if}
		{#if hasMore}
			<Button variant="secondary" class="w-full" disabled={loadingMore} onclick={more}>
				{#if loadingMore}<Spinner />{/if}{moreError ? 'Try again' : 'Load older events'}
			</Button>
		{:else}
			<p class="text-center text-footnote text-muted-foreground">That's everything.</p>
		{/if}
	</div>
{/if}
