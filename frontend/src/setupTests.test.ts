import { describe, expect, it } from 'vitest';

// Verify that the test environment is correctly wired:
//  - jest-dom matchers extend expect (via setupTests.ts)
//  - vitest globals are available
//  - the DOM environment (happy-dom) can create elements and observe attributes

describe('test setup wiring', () => {
    it('provides jest-dom matchers', () => {
        document.body.innerHTML = '<div>hello</div>';
        const el = document.querySelector('div');
        expect(el).toBeInTheDocument();
        expect(el).toBeVisible();
        expect(el).toHaveTextContent('hello');
        expect(el).not.toBeEmptyDOMElement();
    });

    it('supports a basic DOM assertion round-trip', () => {
        const div = document.createElement('div');
        div.setAttribute('role', 'status');
        div.textContent = 'Connected';
        document.body.appendChild(div);
        expect(document.body).toContainHTML('Connected');
        expect(div).toHaveAttribute('role', 'status');
    });

    it('has vitest globals available', () => {
        // describe, it, expect are all available without imports when
        // globals: true is set in vitest config.
        expect(typeof describe).toBe('function');
        expect(typeof it).toBe('function');
        expect(typeof expect).toBe('function');
    });

    it('supports vi mocking primitives', () => {
        const fn = vi.fn((x: number) => x + 1);
        fn(1);
        expect(fn).toHaveBeenCalledWith(1);
        expect(fn).toHaveReturnedWith(2);
    });
});
