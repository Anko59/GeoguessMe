import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import LeaveGroupButton from './LeaveGroupButton';

const mocks = vi.hoisted(() => ({ leave: vi.fn() }));
vi.mock('../../api', () => ({
    groupsAPI: { leave: mocks.leave },
    getAPIErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

beforeEach(() => {
    mocks.leave.mockReset().mockResolvedValue(undefined);
});

function renderButton() {
    return render(
        <MemoryRouter initialEntries={['/group/group-1']}>
            <Routes>
                <Route path="/group/:id" element={<LeaveGroupButton groupId="group-1" />} />
                <Route path="/groups" element={<h1>My Groups</h1>} />
            </Routes>
        </MemoryRouter>,
    );
}

describe('LeaveGroupButton', () => {
    it('leaves only the current group and navigates after success, preventing duplicate requests', async () => {
        let complete!: () => void;
        mocks.leave.mockImplementation(
            () =>
                new Promise<void>((resolve) => {
                    complete = resolve;
                }),
        );
        renderButton();
        const button = screen.getByRole('button', { name: 'Leave the group' });
        fireEvent.click(button);
        fireEvent.click(button);
        expect(mocks.leave).toHaveBeenCalledExactlyOnceWith('group-1');
        expect(screen.getByRole('button', { name: 'Leaving…' })).toBeDisabled();
        expect(screen.queryByRole('heading', { name: 'My Groups' })).not.toBeInTheDocument();
        await act(async () => complete());
        expect(screen.getByRole('heading', { name: 'My Groups' })).toBeInTheDocument();
    });

    it('keeps the group open after failure and allows retry', async () => {
        mocks.leave.mockRejectedValueOnce(new Error('offline'));
        renderButton();
        fireEvent.click(screen.getByRole('button', { name: 'Leave the group' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to leave group. Try again.');
        fireEvent.click(screen.getByRole('button', { name: 'Leave the group' }));
        expect(await screen.findByRole('heading', { name: 'My Groups' })).toBeInTheDocument();
    });

    it('ignores a response after the group settings unmount', async () => {
        let complete!: () => void;
        mocks.leave.mockImplementation(
            () =>
                new Promise<void>((resolve) => {
                    complete = resolve;
                }),
        );
        const view = renderButton();
        fireEvent.click(screen.getByRole('button', { name: 'Leave the group' }));
        view.unmount();
        await act(async () => complete());
        expect(screen.queryByRole('heading', { name: 'My Groups' })).not.toBeInTheDocument();
    });
});
