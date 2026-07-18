import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import TabBar from './TabBar';

describe('TabBar', () => {
    it('changes tabs', () => {
        const onTabChange = vi.fn();
        render(<TabBar activeTab="chat" onTabChange={onTabChange} />);
        fireEvent.click(screen.getByRole('button', { name: /camera/i }));
        fireEvent.click(screen.getByRole('button', { name: /leaderboard/i }));
        expect(onTabChange).toHaveBeenNthCalledWith(1, 'camera');
        expect(onTabChange).toHaveBeenNthCalledWith(2, 'leaderboard');
    });
});
