// Keyboard behavior for radio-style groups (segmented controls, chips):
// arrow keys move and select, Home/End jump, and only the selected item is
// in the tab order (WAI-ARIA radio group pattern).

/** Index to move to for a key press, or null if the key isn't handled. */
export function nextIndex(key: string, current: number, count: number): number | null {
	if (count === 0) return null;
	switch (key) {
		case 'ArrowRight':
		case 'ArrowDown':
			return (current + 1) % count;
		case 'ArrowLeft':
		case 'ArrowUp':
			return (current - 1 + count) % count;
		case 'Home':
			return 0;
		case 'End':
			return count - 1;
		default:
			return null;
	}
}

/**
 * Handles keydown on a radio group container: selects the next enabled
 * item and moves focus to it.
 */
export function onRadioKeydown(e: KeyboardEvent, select: (index: number) => void) {
	const group = e.currentTarget as HTMLElement;
	const items = [...group.querySelectorAll<HTMLElement>('[role="radio"]:not([disabled])')];
	const current = items.indexOf(document.activeElement as HTMLElement);
	const next = nextIndex(e.key, Math.max(0, current), items.length);
	if (next === null) return;
	e.preventDefault();
	items[next].focus();
	select(Number(items[next].dataset.index));
}
