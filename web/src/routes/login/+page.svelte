<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import Inbox from '@lucide/svelte/icons/inbox';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import AuthCard from '#lib/components/lor/AuthCard.svelte';
	import Callout from '#lib/components/lor/Callout.svelte';
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

	const toRequest = $derived(nextPath().startsWith('/r/') || nextPath().startsWith('/e/'));
</script>

<AuthCard title="Sign in to Lor">
	{#snippet subtitle()}Temporary, approved SSH access for your agents.{/snippet}
	<div class="space-y-4">
		{#if toRequest}
			<Callout tone="warning" icon={Inbox} title="A request is waiting">
				You'll go straight to it after signing in.
			</Callout>
		{/if}
		<Button size="lg" class="w-full" onclick={signIn} disabled={busy}>
			{#if busy}<Spinner />{:else}<KeyRound />{/if}
			{busy ? 'Waiting for passkey…' : 'Sign in with passkey'}
		</Button>
	</div>
	{#snippet footer()}Use the passkey you registered for this Lor.{/snippet}
</AuthCard>
