import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Globe from './Globe';
import type { GroupChallenge } from '../../types';

const mocks = vi.hoisted(() => ({
    create: vi.fn(),
    update: vi.fn(),
    focus: vi.fn(),
    rotate: vi.fn(),
    zoom: vi.fn(),
    dispose: vi.fn(),
}));
vi.mock('./globeScene', () => ({ createGlobeScene: mocks.create }));
beforeEach(() => {
    vi.clearAllMocks();
    mocks.create.mockImplementation(
        (_host: HTMLDivElement, _onSelect: unknown, _onError: unknown, onReady?: () => void) => {
            onReady?.();
            return mocks;
        },
    );
});
describe('Globe', () => {
    it('keeps the loading notice until the Earth texture has rendered', async () => {
        let ready: (() => void) | undefined;
        mocks.create.mockImplementationOnce(
            (_host: HTMLDivElement, _onSelect: unknown, _onError: unknown, onReady?: () => void) => {
                ready = onReady;
                return mocks;
            },
        );
        render(<Globe items={[]} selectedID={null} onSelect={vi.fn()} />);
        expect(await screen.findByText('Loading Earth…')).toBeInTheDocument();
        expect(screen.queryByRole('group', { name: 'Globe controls' })).not.toBeInTheDocument();
        act(() => ready?.());
        expect(await screen.findByRole('button', { name: 'Rotate globe left' })).toBeInTheDocument();
        expect(screen.queryByText('Loading Earth…')).not.toBeInTheDocument();
    });

    it('does not recenter an explored globe when another history page arrives', async () => {
        const item: GroupChallenge = {
            photo_id: 'p',
            group_id: 'g',
            user_id: 'u',
            username: 'Alice',
            created_at: '',
            expires_at: '',
            status: 'results',
            lat: 48,
            long: 2,
        };
        const onSelect = vi.fn();
        const view = render(<Globe items={[item]} selectedID="p" onSelect={onSelect} />);
        await waitFor(() => expect(mocks.focus).toHaveBeenCalledWith(item));
        mocks.focus.mockClear();
        fireEvent.click(screen.getByRole('button', { name: 'Rotate globe left' }));
        view.rerender(<Globe items={[item, { ...item, photo_id: 'older' }]} selectedID="p" onSelect={onSelect} />);
        expect(mocks.focus).not.toHaveBeenCalled();
        view.rerender(<Globe items={[item]} selectedID={null} onSelect={onSelect} />);
        view.rerender(<Globe items={[item]} selectedID="p" onSelect={onSelect} />);
        expect(mocks.focus).toHaveBeenCalledOnce();
    });
    it('keeps the scene alive and replays new data when the selection callback changes', async () => {
        const item: GroupChallenge = {
            photo_id: 'p',
            group_id: 'g',
            user_id: 'u',
            username: 'Alice',
            created_at: '',
            expires_at: '',
            status: 'results',
            lat: 48,
            long: 2,
        };
        const firstOnSelect = vi.fn();
        const secondOnSelect = vi.fn();
        let select: ((id: string) => void) | undefined;
        mocks.create.mockImplementationOnce(
            (_host: HTMLDivElement, onSelect: (id: string) => void, _onError: unknown, onReady?: () => void) => {
                select = onSelect;
                onReady?.();
                return mocks;
            },
        );
        const view = render(<Globe items={[]} selectedID={null} onSelect={firstOnSelect} />);
        await waitFor(() => expect(mocks.update).toHaveBeenCalledWith([], null));
        mocks.update.mockClear();

        view.rerender(<Globe items={[item]} selectedID="p" onSelect={secondOnSelect} />);

        await waitFor(() => expect(mocks.update).toHaveBeenCalledWith([item], 'p'));
        expect(mocks.create).toHaveBeenCalledOnce();
        expect(mocks.focus).toHaveBeenCalledWith(item);
        select?.('p');
        expect(secondOnSelect).toHaveBeenCalledWith('p');
    });
    it('offers keyboard accessible rotation and zoom, then disposes on close', async () => {
        const onSelect = vi.fn();
        const view = render(<Globe items={[]} selectedID={null} onSelect={onSelect} />);
        fireEvent.click(await screen.findByRole('button', { name: 'Rotate globe left' }));
        expect(mocks.rotate).toHaveBeenCalledWith(-0.25, 0);
        fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
        expect(mocks.zoom).toHaveBeenCalledWith(0.8);
        await waitFor(() => expect(mocks.update).toHaveBeenCalledWith([], null));
        view.unmount();
        expect(mocks.dispose).toHaveBeenCalledTimes(1);
    });
    it('explains the list fallback when WebGL initialization fails', async () => {
        mocks.create.mockImplementationOnce(() => {
            throw new Error('WebGL unavailable');
        });
        render(<Globe items={[]} selectedID={null} onSelect={vi.fn()} />);
        expect(await screen.findByText(/3D rendering is unavailable/)).toBeInTheDocument();
    });
});
