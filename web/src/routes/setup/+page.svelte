<script lang="ts">
	import { goto } from '$app/navigation';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
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

<div class="mx-auto mt-[12vh] max-w-sm">
	<h1 class="text-2xl">Set up Lor</h1>
	<p class="mt-2 text-muted-foreground">
		On the Lor LXC, run <code>lor-server admin setup-code</code>, then enter the code here and
		register your first passkey.
	</p>
	<form class="mt-8 space-y-5" onsubmit={submit}>
		<div class="space-y-2">
			<Label for="code">Setup code</Label>
			<Input
				id="code"
				bind:value={code}
				placeholder="7KQ-M2P-X9D"
				autocomplete="one-time-code"
				autocapitalize="characters"
				required
			/>
		</div>
		<div class="space-y-2">
			<Label for="name">Passkey name</Label>
			<Input id="name" bind:value={name} placeholder="iPhone" maxlength={64} required />
			<p class="text-sm text-muted-foreground">So you can tell your passkeys apart later.</p>
		</div>
		<Button type="submit" size="lg" class="w-full" disabled={busy || !code || !name}>
			{busy ? 'Waiting for passkey…' : 'Register passkey'}
		</Button>
	</form>
</div>
