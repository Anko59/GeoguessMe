import '@testing-library/jest-dom';
import { afterEach, vi } from 'vitest';

// ---------------------------------------------------------------------------
// Console-output quality gate
// ---------------------------------------------------------------------------
// Unexpected console.warn and console.error cause tests to fail, preventing
// accidental noise from polluting CI logs and hiding real problems. Tests
// that intentionally exercise warning/error paths must opt out by mocking or
// spying on the relevant console method before exercising the code and
// restoring it in an afterEach at the test level.
//
//   // Example: allow a specific warning
//   it('handles a deprecation gracefully', () => {
//       vi.spyOn(console, 'warn').mockImplementation(() => {});
//       triggerDeprecation();
//   });
// ---------------------------------------------------------------------------

const formatConsoleArgs = (args: unknown[]): string =>
    args.map((a) => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ');

const buildConsoleMessage = (level: string, args: unknown[]): string =>
    `Unexpected console.${level}: ${formatConsoleArgs(args)}`;

const consoleReproach = (level: string) => {
    return (...args: unknown[]): never => {
        throw new Error(buildConsoleMessage(level, args));
    };
};

afterEach(() => {
    vi.restoreAllMocks();
});

vi.spyOn(console, 'warn').mockImplementation(consoleReproach('warn'));
vi.spyOn(console, 'error').mockImplementation(consoleReproach('error'));
