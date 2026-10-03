/// <reference types="node" />
import { createRequire } from 'node:module';
import { describe, expect, it } from 'vitest';

interface BracesOptions {
    expand?: boolean;
    escapeInvalid?: boolean;
    maxDepth?: number;
}

interface BracesNode {
    type: string;
    value?: string;
    nodes?: BracesNode[];
    parent?: BracesNode;
}

interface BracesAPI {
    expand(input: string | BracesNode, options?: BracesOptions): string[];
    parse(input: string, options?: BracesOptions): BracesNode;
    compile(input: string | BracesNode, options?: BracesOptions): string;
    stringify(input: string | BracesNode, options?: BracesOptions): string;
}

const require = createRequire(import.meta.url);
const braces = require('braces') as BracesAPI;
const bracesPackage = require('braces/package.json') as { version: string };

function nestedBraces(depth: number) {
    return `${'{'.repeat(depth)}a,b${'}'.repeat(depth)}`;
}

function nestedParentheses(depth: number) {
    return `${'('.repeat(depth)}a${')'.repeat(depth)}`;
}

function nestedAST(depth: number): BracesNode {
    let node: BracesNode = { type: 'text', value: 'leaf' };
    for (let level = 0; level < depth; level++) node = { type: 'brace', nodes: [node] };
    return { type: 'root', nodes: [node] };
}

describe('vendored braces security backport', () => {
    it('resolves the pinned private package and preserves ordinary brace expansion', () => {
        expect(bracesPackage.version).toBe('3.0.4+geoguessme.1');
        expect(braces.expand('map/{north,south}.css')).toEqual(['map/north.css', 'map/south.css']);
    });

    it('bounds nested braces and parentheses at the default depth', () => {
        expect(() => braces.parse(nestedBraces(100))).not.toThrow();
        expect(() => braces.parse(nestedBraces(101))).toThrow(/exceeds max depth/);
        expect(() => braces.parse(nestedParentheses(101))).toThrow(/exceeds max depth/);
    });

    it('honors lower and fractional maxDepth options', () => {
        expect(() => braces.parse('{{a,b},c}', { maxDepth: 2 })).not.toThrow();
        expect(() => braces.parse('{{a,b},c}', { maxDepth: 1 })).toThrow(/exceeds max depth/);
        expect(() => braces.parse('{{a,b},c}', { maxDepth: 1.5 })).toThrow(/exceeds max depth/);
    });

    it('bounds caller-supplied ASTs in every recursive walker', () => {
        const ast = nestedAST(101);
        expect(() => braces.compile(ast)).toThrow(/exceeds max depth/);
        expect(() => braces.expand(ast)).toThrow(/exceeds max depth/);
        expect(() => braces.stringify(ast)).toThrow(/exceeds max depth/);
    });

    it('rejects cyclic AST parent chains without changing stringify behavior', () => {
        const ast: BracesNode = { type: 'paren', nodes: [{ type: 'text', value: 'leaf' }] };
        ast.parent = ast;
        expect(() => braces.expand(ast)).toThrow(/parent chain contains a cycle/);

        for (const pattern of ['{{a}}', '{a,{b}}', '{{x}y}', '{a,{b,{c}}', '{}{a}']) {
            expect(braces.stringify(braces.parse(pattern), { escapeInvalid: true })).toBe(pattern);
        }
    });
});
