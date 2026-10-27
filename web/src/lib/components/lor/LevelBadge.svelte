<script lang="ts">
	// Access level: sudo (root) is always the warning tone.
	import ShieldAlert from '@lucide/svelte/icons/shield-alert';
	import User from '@lucide/svelte/icons/user';
	import { Badge } from '#lib/components/ui/badge/index.js';

	let {
		level,
		size = 'sm',
		short = false
	}: { level: string; size?: 'sm' | 'md'; short?: boolean } = $props();
</script>

{#if level === 'sudo'}
	<Badge tone="warning" {size} data-level="sudo"
		><ShieldAlert aria-hidden="true" />{short ? 'sudo' : 'sudo (root)'}</Badge
	>
{:else}
	<Badge tone="neutral" {size} data-level="user"
		><User aria-hidden="true" />{short ? 'user' : 'Standard user'}</Badge
	>
{/if}
