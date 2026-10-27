// A short, human description of a session's browser: "Safari on iPhone".
// Best effort; falls back to the raw string.

export interface DeviceInfo {
	label: string;
	kind: 'phone' | 'tablet' | 'desktop' | 'unknown';
}

export function describeUserAgent(ua: string): DeviceInfo {
	if (!ua) return { label: 'Unknown browser', kind: 'unknown' };
	const os = /iPhone/.test(ua)
		? 'iPhone'
		: /iPad/.test(ua)
			? 'iPad'
			: /Android/.test(ua)
				? 'Android'
				: /Mac OS X|Macintosh/.test(ua)
					? 'macOS'
					: /Windows/.test(ua)
						? 'Windows'
						: /CrOS/.test(ua)
							? 'ChromeOS'
							: /Linux/.test(ua)
								? 'Linux'
								: null;
	const browser = /Edg\//.test(ua)
		? 'Edge'
		: /Firefox\/|FxiOS/.test(ua)
			? 'Firefox'
			: /CriOS|Chrome\/|Chromium/.test(ua)
				? /HeadlessChrome/.test(ua)
					? 'Headless Chrome'
					: 'Chrome'
				: /Safari\//.test(ua)
					? 'Safari'
					: null;
	const kind: DeviceInfo['kind'] =
		os === 'iPhone' || (os === 'Android' && /Mobile/.test(ua))
			? 'phone'
			: os === 'iPad' || os === 'Android'
				? 'tablet'
				: os
					? 'desktop'
					: 'unknown';
	if (!os && !browser) return { label: ua, kind };
	return { label: [browser, os].filter(Boolean).join(' on '), kind };
}
