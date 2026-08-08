import '@testing-library/jest-dom';
import { afterEach, beforeEach, vi } from 'vitest';

// ---------------------------------------------------------------------------
// Console-output quality gate
// ---------------------------------------------------------------------------
// Genuine React development warnings must fail tests instead of being
// swallowed as CI noise: an act() violation, a state update during render, or
// a suspended resource finishing outside act() indicates a broken test or
// component contract. Ordinary application logging is forwarded unchanged so
// tests that intentionally exercise warning/error paths keep working.
//
// A test can still opt out for a specific case by mocking the relevant console
// method before exercising the code:
//
//   it('handles a deprecation gracefully', () => {
//       vi.spyOn(console, 'warn').mockImplementation(() => {});
//       triggerDeprecation();
//   });
// ---------------------------------------------------------------------------

// Stable, dev-only React diagnostic prefixes confirmed against the pinned
// react-dom-client.development.js (React 19.2.8). Each marker must remain a
// genuine React dev warning; re-validate these on any React version bump.
const REACT_DEV_WARNING_MARKERS = [
    'was not wrapped in act(',
    'A suspended resource finished loading inside a test',
    'Cannot update a component',
] as const;

// Capture the real methods once: the gate always forwards through these, so
// re-installing the gate can never wrap an already-gated console method.
const originalConsoleError = console.error;
const originalConsoleWarn = console.warn;

const formatConsoleArgs = (args: unknown[]): string =>
    args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ');

const isReactDevWarning = (args: unknown[]): boolean => {
    const first = args[0];
    const text = typeof first === 'string' ? first : JSON.stringify(first);
    return REACT_DEV_WARNING_MARKERS.some((marker) => text.includes(marker));
};

const installConsoleGate = (): void => {
    console.error = (...args: unknown[]) => {
        if (isReactDevWarning(args)) {
            throw new Error(`Unexpected console.error: ${formatConsoleArgs(args)}`);
        }
        originalConsoleError(...args);
    };
    console.warn = (...args: unknown[]) => {
        if (isReactDevWarning(args)) {
            throw new Error(`Unexpected console.warn: ${formatConsoleArgs(args)}`);
        }
        originalConsoleWarn(...args);
    };
};

installConsoleGate();

// vi.restoreAllMocks() strips a vi.spyOn-based gate after the first test of
// each file. Re-install on both sides of every test so the gate is armed for
// every test and survives mock restoration, regardless of whether the test
// mocked a console method.
afterEach(() => {
    vi.restoreAllMocks();
    installConsoleGate();
});

beforeEach(() => {
    installConsoleGate();
});
