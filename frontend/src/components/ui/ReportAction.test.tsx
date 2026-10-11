import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import ReportAction from './ReportAction';

const mocks = vi.hoisted(() => ({ report: vi.fn() }));
vi.mock('../../api', () => ({
    moderationAPI: { report: mocks.report },
    getAPIErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

beforeEach(() => mocks.report.mockReset());

it('submits a message notice and displays the receipt without retaining details', async () => {
    mocks.report.mockResolvedValue({ id: 'report-1' });
    render(<ReportAction kind="messages" targetID="message-1" />);
    fireEvent.click(screen.getByRole('button', { name: 'Report message' }));
    fireEvent.change(screen.getByRole('combobox', { name: 'Reason' }), { target: { value: 'harassment' } });
    fireEvent.change(screen.getByRole('textbox', { name: 'Details (optional)' }), {
        target: { value: 'A description' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Submit report' }));
    await waitFor(() =>
        expect(mocks.report).toHaveBeenCalledWith(
            'messages',
            'message-1',
            { reason: 'harassment', details: 'A description' },
            expect.any(AbortSignal),
        ),
    );
    expect(await screen.findByRole('status')).toHaveTextContent('report-1');
    expect(screen.queryByDisplayValue('A description')).not.toBeInTheDocument();
});

it('requires an explanation for an illegality notice', () => {
    render(<ReportAction kind="messages" targetID="message-1" />);
    fireEvent.click(screen.getByRole('button', { name: 'Report message' }));
    fireEvent.change(screen.getByRole('combobox', { name: 'Reason' }), { target: { value: 'illegal_content' } });
    expect(screen.getByRole('textbox', { name: 'Details (required)' })).toBeRequired();
});
