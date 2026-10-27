<script lang="ts">
	// A grant or extension status as a tinted badge with a dot. Carries
	// data-status for tests and styling hooks.
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { extensionLook, grantLook } from '#lib/status.js';
	import StatusDot from './StatusDot.svelte';

	let {
		status,
		kind = 'grant',
		size = 'sm'
	}: { status: string; kind?: 'grant' | 'extension'; size?: 'sm' | 'md' } = $props();
	const look = $derived(kind === 'grant' ? grantLook(status) : extensionLook(status));
</script>

<Badge tone={look.tone} {size} data-status={status}>
	<StatusDot tone={look.tone} pulse={status === 'pending'} class="size-1.5" />
	{look.label}
</Badge>
