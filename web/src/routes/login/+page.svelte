<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { loginWithPasskey } from '#lib/webauthn.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { keys } from '#lib/queries.js';

	const qc = useQueryClient();
	let busy = $state(false);

	// Only same-app paths are allowed as the post-login destination.
	function nextPath(): string {
		const n = page.url.searchParams.get('next') ?? '/';
		return n.startsWith('/') && !n.startsWith('//') && !n.startsWith('/\\') ? n : '/';
	}

	async function signIn() {
		busy = true;
		try {
			const s = await loginWithPasskey();
			qc.setQueryData(keys.session, s);
			await goto(nextPath(), { replaceState: true });
		} catch (e) {
			if (!isCancel(e)) toast.error(errorMessage(e));
		} finally {
			busy = false;
		}
	}
</script>

<div class="mx-auto mt-[18vh] max-w-sm text-center">
	<h1 class="text-3xl">Lor</h1>
	<p class="mt-2 text-muted-foreground">Sign in with your passkey.</p>
	<Button size="lg" class="mt-8 w-full" onclick={signIn} disabled={busy}>
		<KeyRound />
		{busy ? 'Waiting for passkey…' : 'Sign in with passkey'}
	</Button>
	{#if nextPath().startsWith('/r/') || nextPath().startsWith('/e/')}
		<p class="mt-4 text-sm text-muted-foreground">
			You'll go straight to the request after signing in.
		</p>
	{/if}
</div>
