import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mocks = vi.hoisted(() => ({
    installable: false,
    isIosSafari: false,
    promptInstall: vi.fn(),
}));

vi.mock('./usePwaInstall', async (importOriginal) => {
    const actual = (await importOriginal()) as Record<string, unknown>;
    return {
        ...actual,
        isStandaloneDisplay: () => false,
        isIosSafari: () => mocks.isIosSafari,
        usePwaInstall: () => ({
            installable: mocks.installable,
            installed: false,
            dismissed: false,
            promptInstall: mocks.promptInstall,
            dismiss: vi.fn(),
        }),
    };
});

import MobileInstallPopup from './MobileInstallPopup';

beforeEach(() => {
    sessionStorage.clear();
    mocks.installable = false;
    mocks.isIosSafari = false;
    mocks.promptInstall.mockReset();
    // Simulate a mobile device via pointer media query.
    window.matchMedia = ((query: string) => ({
        matches: query === '(pointer: coarse)',
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(() => true),
    })) as typeof window.matchMedia;
});

describe('MobileInstallPopup', () => {
    it('shows on mobile devices', () => {
        render(<MobileInstallPopup />);
        expect(screen.getByText(/Add GeoGuessMe to your home screen/)).toBeInTheDocument();
    });

    it('does not show on desktop', () => {
        window.matchMedia = ((query: string) => ({
            matches: false,
            media: query,
            onchange: null,
            addListener: vi.fn(),
            removeListener: vi.fn(),
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
            dispatchEvent: vi.fn(() => true),
        })) as typeof window.matchMedia;
        const { container } = render(<MobileInstallPopup />);
        expect(container.firstChild).toBeNull();
    });

    it('dismiss button hides the popup', async () => {
        render(<MobileInstallPopup />);
        await userEvent.click(screen.getByLabelText('Close'));
        expect(screen.queryByText(/Add GeoGuessMe to your home screen/)).not.toBeInTheDocument();
    });

    it('does not show when previously dismissed', () => {
        sessionStorage.setItem('geoguessme:pwa-popup-dismissed', '1');
        const { container } = render(<MobileInstallPopup />);
        expect(container.firstChild).toBeNull();
    });

    it('offers the native install button on Android when available', async () => {
        mocks.installable = true;
        mocks.promptInstall.mockResolvedValue('accepted');
        render(<MobileInstallPopup />);

        await userEvent.click(screen.getByRole('button', { name: 'Install app' }));

        expect(mocks.promptInstall).toHaveBeenCalledOnce();
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('keeps the Safari installation instructions', () => {
        mocks.isIosSafari = true;
        render(<MobileInstallPopup />);

        expect(screen.getByText(/Choose/)).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Install app' })).not.toBeInTheDocument();
    });
});
