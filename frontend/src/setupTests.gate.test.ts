import { afterEach, describe, expect, it, vi } from 'vitest';

// Regression coverage for the console quality gate installed in setupTests.ts:
//  - React development warnings (act() violations, setState-in-render) fail tests
//  - ordinary application logging is forwarded and does not fail tests
//  - the gate stays armed across tests even after vi.restoreAllMocks()

const reactActMarker = 'An update to TestComponent inside a test was not wrapped in act(...).';
const reactRenderMarker = 'Cannot update a component (`Header`) while rendering a different component (`Body`).';
const appLogMessage = 'application log: upload failed (expected in this test)';

describe('console output quality gate', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('fails on a React act() warning emitted via console.error', () => {
        expect(() => console.error(reactActMarker)).toThrow('Unexpected console.error');
    });

    it('fails on a React act() warning emitted via console.warn', () => {
        expect(() => console.warn(reactActMarker)).toThrow('Unexpected console.warn');
    });

    it('fails on a React setState-in-render warning', () => {
        expect(() => console.error(reactRenderMarker)).toThrow('Unexpected console.error');
    });

    it('forwards ordinary application console.error output without failing', () => {
        expect(() => console.error(appLogMessage)).not.toThrow();
        // the gate must remain armed after forwarding
        expect(() => console.error(reactActMarker)).toThrow('Unexpected console.error');
    });

    it('forwards ordinary application console.warn output without failing', () => {
        expect(() => console.warn(appLogMessage)).not.toThrow();
    });

    it('stays armed in a later test after vi.restoreAllMocks()', () => {
        // earlier tests in this file run the afterEach(vi.restoreAllMocks());
        // the gate must still be active here
        expect(() => console.error(reactActMarker)).toThrow('Unexpected console.error');
    });
});
