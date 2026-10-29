// Accept a grant number pasted with or without its usual # prefix.
export function parseGrantFilter(input: string): { value?: number; error?: string } {
	const text = input.trim();
	if (!text) return {};
	const digits = text.replace(/^#/, '');
	const value = Number(digits);
	if (!/^[0-9]+$/.test(digits) || !Number.isSafeInteger(value) || value <= 0) {
		return { error: 'Enter a positive grant number, such as 7 or #7.' };
	}
	return { value };
}
