import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import GroupRail from '../GroupRail';

const inbox = vi.hoisted(() => vi.fn());
vi.mock('../../../api', () => ({
    groupsAPI: { inbox, markRead: vi.fn() },
    getAPIErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

describe('GroupRail', () => {
    beforeEach(() => {
        vi.resetAllMocks();
    });

    it('renders group identity, latest metadata, and the server unread count', async () => {
        inbox.mockResolvedValue([
            {
                id: 'group-1',
                name: 'Paris crew',
                unread_count: 4,
                latest_message: {
                    id: 'message-1',
                    kind: 'text',
                    username: 'Alice',
                    created_at: '2026-09-18T10:00:00Z',
                },
            },
        ]);
        render(
            <MemoryRouter>
                <GroupRail />
            </MemoryRouter>,
        );
        const link = await screen.findByRole('link', { name: /Paris crew.*Alice.*text.*4 unread/i });
        expect(link).toHaveAttribute('href', '/group/group-1');
        expect(screen.getByLabelText('4 unread messages')).toBeInTheDocument();
    });

    it('has an explicit empty state when the user has no groups', async () => {
        inbox.mockResolvedValue([]);
        render(
            <MemoryRouter>
                <GroupRail />
            </MemoryRouter>,
        );
        expect(await screen.findByText('No groups yet.')).toBeInTheDocument();
        expect(screen.getByRole('link', { name: 'Find your groups →' })).toHaveAttribute('href', '/groups');
    });
});
