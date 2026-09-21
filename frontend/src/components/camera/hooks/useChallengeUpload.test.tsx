import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useChallengeOptions } from '../useChallengeUpload';

describe('useChallengeOptions', () => {
    it('resets challenge options when navigating to another group', () => {
        const hook = renderHook(({ groupID }) => useChallengeOptions(groupID), {
            initialProps: { groupID: 'group-1' },
        });
        act(() => {
            hook.result.current.toggleGroup('group-2');
            hook.result.current.toggleHideLocation();
        });
        expect(hook.result.current.targetGroupIDs).toEqual(['group-1', 'group-2']);
        expect(hook.result.current.hideLocation).toBe(true);

        hook.rerender({ groupID: 'group-2' });
        expect(hook.result.current.targetGroupIDs).toEqual(['group-2']);
        expect(hook.result.current.hideLocation).toBe(false);
    });

    it('starts feed captures without a group and keeps feed metadata in camera options', () => {
        const hook = renderHook(() => useChallengeOptions('', true));
        expect(hook.result.current.targetGroupIDs).toEqual([]);
        expect(hook.result.current.audience).toBe('public');
        expect(hook.result.current.caption).toBe('');
        act(() => {
            hook.result.current.setAudience('friends');
            hook.result.current.setCaption('A clue');
        });
        expect(hook.result.current.audience).toBe('friends');
        expect(hook.result.current.caption).toBe('A clue');
    });
});
