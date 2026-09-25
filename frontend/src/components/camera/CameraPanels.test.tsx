import { render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CameraOptionsMenu } from './CameraPanels';

vi.mock('../../pages/groups/groupPhotoCache', () => ({
    useGroupPhotoUrl: () => '/logo.png',
}));

describe('CameraOptionsMenu', () => {
    it('keeps feed visibility controls as compact, labelled radio inputs', () => {
        render(
            <CameraOptionsMenu
                groups={[]}
                selectedGroupIDs={[]}
                hideLocation={false}
                onToggleGroup={vi.fn()}
                onToggleHideLocation={vi.fn()}
                feedMode
                feedAudience="public"
                onAudienceChange={vi.fn()}
                onCaptionChange={vi.fn()}
                onClose={vi.fn()}
            />,
        );

        const audience = screen.getByRole('group', { name: 'Feed visibility' });
        const radios = within(audience).getAllByRole('radio');
        expect(radios).toHaveLength(2);
        expect(radios.map((radio) => radio.getAttribute('type'))).toEqual(['radio', 'radio']);
        expect(within(audience).getByRole('radio', { name: 'Public feed' })).toBeChecked();
        expect(within(audience).getByRole('radio', { name: 'Friends only' })).not.toBeChecked();
        expect(within(audience).getAllByText(/Public feed|Friends only/)).toHaveLength(2);
    });
});
