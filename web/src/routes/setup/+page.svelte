<script lang="ts">
	import KeyRound from '@lucide/svelte/icons/key-round';
	import { goto } from '$app/navigation';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Spinner } from '#lib/components/ui/spinner/index.js';
	import AuthCard from '#lib/components/lor/AuthCard.svelte';
	import Card from '#lib/components/lor/Card.svelte';
	import Field from '#lib/components/lor/Field.svelte';
	import { setupPasskey } from '#lib/webauthn.js';
	import { errorMessage, isCancel } from '#lib/errors.js';
	import { keys } from '#lib/queries.js';

	const qc = useQueryClient();
	let code = $state('');
	let name = $state('');
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const s = await setupPasskey(code, name);
			qc.setQueryData(keys.session, s);
			toast.success('Passkey registered. You are signed in.');
			await goto('/', { replaceState: true });
		} catch (err) {
			if (!isCancel(err)) toast.error(errorMessage(err));
		} finally {
			busy = false;
		}
	}
</script>

<AuthCard title="Set up Lor">
	{#snippet subtitle()}
		On the Lor LXC, run <code class="text-foreground">lor-server admin setup-code</code>, then enter
		the code here and register your first passkey.
	{/snippet}
	<form onsubmit={submit} class="space-y-4">
		<Card class="space-y-5">
			<Field label="Setup code" for="code" hint="Works once, for a short time.">
				<Input
					id="code"
					bind:value={code}
					placeholder="7KQ-M2P-X9D"
					autocomplete="one-time-code"
					autocapitalize="characters"
					class="font-mono tracking-wider"
					required
				/>
			</Field>
			<Field label="Passkey name" for="name" hint="So you can tell your passkeys apart later.">
				<Input id="name" bind:value={name} placeholder="iPhone" maxlength={64} required />
			</Field>
		</Card>
		<Button type="submit" size="lg" class="w-full" disabled={busy || !code || !name}>
			{#if busy}<Spinner />{:else}<KeyRound />{/if}
			{busy ? 'Waiting for passkey…' : 'Register passkey'}
		</Button>
	</form>
</AuthCard>
