// The consent page's duration picker (plan section 11): 15m, 30m, 1h, 2h,
// 4h, 8h, 1d, 3d, 1w, or custom. Over 24h shows a note but isn't blocked.

export interface DurationChoice {
	label: string;
	seconds: number;
}

export const DURATION_CHOICES: DurationChoice[] = [
	{ label: '15 min', seconds: 15 * 60 },
	{ label: '30 min', seconds: 30 * 60 },
	{ label: '1 h', seconds: 3600 },
	{ label: '2 h', seconds: 2 * 3600 },
	{ label: '4 h', seconds: 4 * 3600 },
	{ label: '8 h', seconds: 8 * 3600 },
	{ label: '1 day', seconds: 86400 },
	{ label: '3 days', seconds: 3 * 86400 },
	{ label: '1 week', seconds: 7 * 86400 }
];

/** Extension shortcuts on the Extend button. */
export const EXTEND_CHOICES: DurationChoice[] = [
	{ label: '+30 min', seconds: 30 * 60 },
	{ label: '+1 h', seconds: 3600 },
	{ label: '+4 h', seconds: 4 * 3600 },
	{ label: '+1 day', seconds: 86400 }
];

export const LONG_DURATION_S = 86400;

/** Whether a duration should show the "longer than a day" note. */
export function isLong(seconds: number): boolean {
	return seconds > LONG_DURATION_S;
}

/**
 * Parses a custom duration like "90m", "2h", "1h30m", "3d", "1w", "45"
 * (minutes). Returns seconds, or null if invalid.
 */
export function parseDuration(input: string): number | null {
	const s = input.trim().toLowerCase().replace(/\s+/g, '');
	if (s === '') return null;
	if (/^\d+$/.test(s)) {
		const n = Number(s) * 60;
		return n > 0 ? n : null;
	}
	const re = /(\d+(?:\.\d+)?)(w|d|h|mins|min|m|s)/g;
	let total = 0;
	let consumed = 0;
	for (const m of s.matchAll(re)) {
		if (m.index !== consumed) return null;
		consumed += m[0].length;
		const n = Number(m[1]);
		const unit = m[2];
		const mult =
			unit === 'w' ? 604800 : unit === 'd' ? 86400 : unit === 'h' ? 3600 : unit === 's' ? 1 : 60;
		total += n * mult;
	}
	if (consumed !== s.length || total <= 0) return null;
	return Math.round(total);
}

/** The choice matching seconds, if it's one of the presets. */
export function choiceFor(seconds: number): DurationChoice | undefined {
	return DURATION_CHOICES.find((c) => c.seconds === seconds);
}

/** Clamps a duration to [min, max] seconds; null if out of range. */
export function validDuration(seconds: number | null, min: number, max: number): boolean {
	return seconds !== null && seconds >= min && seconds <= max;
}
