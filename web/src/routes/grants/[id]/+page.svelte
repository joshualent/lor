<script lang="ts">
	import { page } from '$app/state';
	import GrantView from '#lib/components/lor/GrantView.svelte';
	const id = $derived(Number(page.params.id));
</script>

{#key id}<GrantView {id} />{/key}
