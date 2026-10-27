// Time and duration formatting. Pure functions (unit-tested).

const MIN = 60;
const HOUR = 3600;
const DAY = 86400;

/** Compact duration: "15 min", "1 h", "1 h 30 min", "3 days", "1 week". */
export function formatDuration(seconds: number): string {
	const s = Math.max(0, Math.round(seconds));
	if (s % (7 * DAY) === 0 && s >= 7 * DAY) {
		const w = s / (7 * DAY);
		return w === 1 ? '1 week' : `${w} weeks`;
	}
	if (s % DAY === 0 && s >= DAY) {
		const d = s / DAY;
		return d === 1 ? '1 day' : `${d} days`;
	}
	if (s >= DAY) {
		const d = Math.floor(s / DAY);
		const h = Math.round((s % DAY) / HOUR);
		return h ? `${d} d ${h} h` : `${d} d`;
	}
	if (s >= HOUR) {
		const h = Math.floor(s / HOUR);
		const m = Math.round((s % HOUR) / MIN);
		return m ? `${h} h ${m} min` : `${h} h`;
	}
	if (s >= MIN) return `${Math.round(s / MIN)} min`;
	return `${s} s`;
}

/**
 * Countdown to an end time, ticking every second near the end:
 * "2 d 4 h", "3 h 05 min", "12 min", "4:09", "0:07", or "ended".
 */
export function formatCountdown(endMs: number, nowMs: number): string {
	const left = Math.floor((endMs - nowMs) / 1000);
	if (left <= 0) return 'ended';
	if (left >= DAY) {
		const d = Math.floor(left / DAY);
		const h = Math.floor((left % DAY) / HOUR);
		return `${d} d ${h} h`;
	}
	if (left >= HOUR) {
		const h = Math.floor(left / HOUR);
		const m = Math.floor((left % HOUR) / MIN);
		return `${h} h ${String(m).padStart(2, '0')} min`;
	}
	if (left >= 10 * MIN) return `${Math.floor(left / MIN)} min`;
	const m = Math.floor(left / MIN);
	const sec = left % MIN;
	return `${m}:${String(sec).padStart(2, '0')}`;
}

/** "2 min ago", "just now", "3 h ago", "Oct 24". */
export function formatAgo(thenMs: number, nowMs: number): string {
	const s = Math.floor((nowMs - thenMs) / 1000);
	if (s < 45) return 'just now';
	if (s < HOUR) return `${Math.max(1, Math.round(s / MIN))} min ago`;
	if (s < DAY) return `${Math.round(s / HOUR)} h ago`;
	if (s < 7 * DAY) return `${Math.round(s / DAY)} d ago`;
	return new Date(thenMs).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/**
 * Clock time with the time zone, plus the date when it isn't today:
 * "14:45 EDT" or "Oct 26, 14:45 EDT".
 */
export function formatClock(ms: number, nowMs: number, timeZone?: string): string {
	const d = new Date(ms);
	const n = new Date(nowMs);
	const time = d.toLocaleTimeString(undefined, {
		hour: '2-digit',
		minute: '2-digit',
		timeZoneName: 'short',
		timeZone
	});
	if (d.toDateString() === n.toDateString()) return time;
	return `${d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone })}, ${time}`;
}

/** Full timestamp for logs, with the time zone. */
export function formatTimestamp(ms: number): string {
	return new Date(ms).toLocaleString(undefined, {
		month: 'short',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		timeZoneName: 'short'
	});
}

function hhmm(ms: number, timeZone?: string): string {
	const time = new Date(ms).toLocaleTimeString('en-GB', {
		hour: '2-digit',
		minute: '2-digit',
		timeZone
	});
	const zone =
		new Intl.DateTimeFormat('en-US', { timeZoneName: 'short', timeZone })
			.formatToParts(ms)
			.find((p) => p.type === 'timeZoneName')?.value ?? '';
	return zone ? `${time} ${zone}` : time;
}

/**
 * The line to paste into chat after creating an offer. The agent's machine
 * may be in another time zone, so the end time carries the admin's zone and
 * UTC: "until 15:51 EDT (19:51 UTC)".
 */
export function offerChatLine(
	host: string,
	level: string,
	endMs: number,
	timeZone?: string
): string {
	const local = hhmm(endMs, timeZone);
	const utc = hhmm(endMs, 'UTC');
	const when = local === utc ? utc : `${local} (${utc})`;
	const lvl = level === 'sudo' ? ' (sudo)' : '';
	return `You have Lor access to \`${host}\`${lvl} until ${when}. Use \`lor ssh ${host} -- <cmd>\`.`;
}

/**
 * Renders a reported command. Like ssh, `lor ssh` joins the arguments with
 * spaces and the remote shell runs the result, so that's what is shown.
 * [] is an interactive shell.
 */
export function formatArgv(argv: string[]): string {
	if (argv.length === 0) return '(interactive shell)';
	return argv.join(' ');
}

/** Time of day for rows already grouped by day: "14:05" / "2:05 PM". */
export function formatTimeOfDay(ms: number): string {
	return new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
