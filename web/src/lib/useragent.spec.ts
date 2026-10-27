import { describe, expect, it } from 'vitest';
import { describeUserAgent } from './useragent';

describe('describeUserAgent', () => {
	it.each([
		[
			'Mozilla/5.0 (iPhone; CPU iPhone OS 19_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.1 Mobile/15E148 Safari/604.1',
			'Safari on iPhone',
			'phone'
		],
		[
			'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36',
			'Chrome on macOS',
			'desktop'
		],
		[
			'Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0',
			'Firefox on Linux',
			'desktop'
		],
		[
			'Mozilla/5.0 (Linux; Android 16; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0 Mobile Safari/537.36',
			'Chrome on Android',
			'phone'
		]
	])('%s', (ua, label, kind) => {
		expect(describeUserAgent(ua)).toEqual({ label, kind });
	});
	it('falls back for empty and unknown strings', () => {
		expect(describeUserAgent('')).toEqual({ label: 'Unknown browser', kind: 'unknown' });
		expect(describeUserAgent('curl/8.0').label).toBe('curl/8.0');
	});
});
