import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Globe from './Globe';

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
    mocks.create.mockReturnValue(mocks);
});
describe('Globe', () => {
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
