/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const profileStyles = readFileSync('src/pages/profile/ProfilePage.css', 'utf8');
const leaderboardStyles = readFileSync('src/pages/profile/FeedLeaderboard.css', 'utf8');
const mapPinStyles = readFileSync('src/components/profile/MapPinProfileCard.css', 'utf8');

// Happy DOM cannot measure overflow. Guard the sizing/wrapping contracts here; the
// real 320/390px viewport journey remains the geometry regression gate.
function stylesheetRules(source: string) {
    const style = document.createElement('style');
    style.textContent = source;
    document.head.append(style);
    const rules = Array.from(style.sheet!.cssRules);
    style.remove();
    return rules;
}

function declarations(rules: CSSRule[], selector: string) {
    const normalize = (value: string) => value.replace(/\s+/g, ' ').trim();
    const rule = rules.find(
        (rule) => 'selectorText' in rule && normalize(String(rule.selectorText)) === normalize(selector),
    ) as CSSStyleRule | undefined;
    expect(rule, selector).toBeDefined();
    return rule!.style;
}

const profileRules = stylesheetRules(profileStyles);
const leaderboardRules = stylesheetRules(leaderboardStyles);
const mapPinRules = stylesheetRules(mapPinStyles);

describe('Profile layout sizing contracts', () => {
    it('constrains the page grid without clipping or hiding overflowing content', () => {
        const page = declarations(profileRules, '.profile-page');
        expect(page.getPropertyValue('grid-template-columns')).toBe('minmax(0, 1fr)');
        expect(page.getPropertyValue('overflow')).toBe('');
        expect(declarations(profileRules, '.profile-page > *').getPropertyValue('min-width')).toBe('0');
    });

    it('keeps identity, rank, next rank and pin text readable by wrapping', () => {
        for (const selector of ['.profile-hero h1', '.profile-rank-label', '.profile-next-rank-copy h2']) {
            expect(declarations(profileRules, selector).getPropertyValue('overflow-wrap')).toBe('anywhere');
        }
        expect(declarations(profileRules, '.profile-rank-label').getPropertyValue('min-width')).toBe('0');
        expect(declarations(mapPinRules, '.profile-map-pin__details').getPropertyValue('overflow-wrap')).toBe(
            'anywhere',
        );
    });

    it('removes the long leaderboard heading and scope from a single inflexible row', () => {
        expect(
            declarations(leaderboardRules, '.profile-feed-leaderboard .leaderboard-header').getPropertyValue(
                'grid-template-columns',
            ),
        ).toBe('auto minmax(0, 1fr)');
        const scope = declarations(leaderboardRules, '.profile-feed-leaderboard .leaderboard-scope');
        expect(scope.getPropertyValue('grid-column')).toBe('2');
        expect(scope.getPropertyValue('white-space')).toBe('normal');
        expect(declarations(leaderboardRules, '.profile-feed-leaderboard').getPropertyValue('overflow-wrap')).toBe(
            'anywhere',
        );
    });

    it('gives mobile leaderboard names their own row without truncating usernames', () => {
        const mobile = leaderboardRules.find(
            (rule) => 'conditionText' in rule && rule.conditionText === '(max-width: 520px)',
        ) as CSSMediaRule;
        expect(mobile).toBeDefined();
        const info = declarations(Array.from(mobile.cssRules), '.profile-feed-leaderboard .entry-info');
        expect(info.getPropertyValue('grid-row')).toBe('2');
        expect(info.getPropertyValue('grid-column')).toBe('2 / -1');
        const username = declarations(
            leaderboardRules,
            '.profile-feed-leaderboard .entry-username,\n.profile-feed-leaderboard .entry-username-link',
        );
        expect(username.getPropertyValue('white-space')).toBe('normal');
        expect(username.getPropertyValue('overflow')).toBe('visible');
    });
});
