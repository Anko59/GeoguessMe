import { act, render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { describe, expect, it, vi } from 'vitest';
import BlockVisibilityBoundary from './BlockVisibilityBoundary';
import { BLOCK_VISIBILITY_KEY, notifyBlockVisibilityChanged } from '../utils/blockVisibility';

const mocks = vi.hoisted(() => ({ avatars: vi.fn(), groups: vi.fn(), leaderboard: vi.fn() }));
vi.mock('../components/common/avatarCache', () => ({ clearAvatarCache: mocks.avatars }));
vi.mock('../pages/groups/groupPhotoCache', () => ({ clearGroupPhotoCache: mocks.groups }));
vi.mock('../components/leaderboard/leaderboardCache', () => ({ clearLeaderboardCache: mocks.leaderboard }));

describe('BlockVisibilityBoundary', () => {
    it('purges caches and remounts private views for local and cross-tab changes, then unsubscribes', () => {
        localStorage.setItem('geoguessme:pwa-session:v1', JSON.stringify({ id: 'viewer' }));
        localStorage.setItem('geoguessme:pwa-messages:v1:viewer:g', 'private history');
        localStorage.setItem('geoguessme:pwa-messages:v1:old-viewer:g', 'old private history');
        localStorage.setItem('unrelated', 'keep');
        let mounts = 0;
        const cleanup = vi.fn();
        function PrivateView() {
            useEffect(() => {
                mounts += 1;
                return cleanup;
            }, []);
            return <p>Private content</p>;
        }
        const view = render(
            <BlockVisibilityBoundary>
                <PrivateView />
            </BlockVisibilityBoundary>,
        );
        act(() => notifyBlockVisibilityChanged());
        expect(mounts).toBe(2);
        expect(cleanup).toHaveBeenCalledTimes(1);
        expect(mocks.avatars).toHaveBeenCalledTimes(1);
        expect(mocks.groups).toHaveBeenCalledTimes(1);
        expect(mocks.leaderboard).toHaveBeenCalledTimes(1);
        expect(localStorage.getItem('geoguessme:pwa-session:v1')).toBeNull();
        expect(localStorage.getItem('geoguessme:pwa-messages:v1:viewer:g')).toBeNull();
        expect(localStorage.getItem('geoguessme:pwa-messages:v1:old-viewer:g')).toBeNull();
        expect(localStorage.getItem('unrelated')).toBe('keep');
        act(() => window.dispatchEvent(new StorageEvent('storage', { key: 'unrelated' })));
        expect(mounts).toBe(2);
        act(() => window.dispatchEvent(new StorageEvent('storage', { key: BLOCK_VISIBILITY_KEY })));
        expect(mounts).toBe(3);
        expect(screen.getByText('Private content')).toBeInTheDocument();
        view.unmount();
        act(() => notifyBlockVisibilityChanged());
        expect(mocks.avatars).toHaveBeenCalledTimes(2);
        localStorage.clear();
    });
});
