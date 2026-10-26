// A shared clock that ticks every second, for countdowns.
let now = $state(Date.now());
let started = false;

export const clock = {
	get now() {
		if (!started && typeof window !== 'undefined') {
			started = true;
			setInterval(() => (now = Date.now()), 1000);
		}
		return now;
	}
};
