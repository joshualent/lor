<script lang="ts">
	// Grant status as calm text with a small dot, not a pill.
	let { status }: { status: string } = $props();
	const LABELS: Record<string, string> = {
		pending: 'Waiting for approval',
		offered: 'Offered',
		active: 'Active',
		denied: 'Denied',
		expired: 'Expired',
		released: 'Released',
		revoked: 'Revoked',
		request_timeout: 'Request timed out',
		offer_expired: 'Offer expired',
		approved: 'Approved',
		moot: 'Moot (grant ended)'
	};
	const tone = $derived(
		status === 'active' || status === 'approved'
			? 'bg-success'
			: status === 'pending' || status === 'offered'
				? 'bg-warning'
				: 'bg-muted-foreground/60'
	);
</script>

<span class="inline-flex items-center gap-2" data-status={status}>
	<span class="size-2 rounded-full {tone}" aria-hidden="true"></span>
	{LABELS[status] ?? status}
</span>
