import { useState } from 'react';
import type { TextBanner, TextBannerStyle } from './textBanner';

interface TextBannerEditorProps {
    banner: TextBanner;
    onChange: (banner: TextBanner) => void;
}

const STYLE_OPTIONS: Array<{ id: TextBannerStyle; label: string }> = [
    { id: 'classic', label: 'Classic' },
    { id: 'neon', label: 'Neon' },
    { id: 'clean', label: 'Clean' },
];

export function TextBannerOverlay({ banner }: { banner: TextBanner }) {
    if (!banner.text.trim()) return null;
    return (
        <div
            className={`camera-text-banner camera-text-banner-${banner.style}`}
            style={{ top: `${banner.position}%` }}
            aria-hidden="true"
        >
            <span>{banner.text}</span>
        </div>
    );
}

export default function TextBannerEditor({ banner, onChange }: TextBannerEditorProps) {
    const [open, setOpen] = useState(false);

    return (
        <div className={`text-banner-editor ${open ? 'open' : ''}`}>
            <button
                type="button"
                className="text-banner-toggle"
                aria-expanded={open}
                aria-controls="camera-text-banner-controls"
                onClick={() => setOpen((current) => !current)}
            >
                <span aria-hidden="true">Aa</span>
                Text
            </button>
            {open && (
                <div id="camera-text-banner-controls" className="text-banner-controls">
                    <label>
                        <span className="sr-only">Photo banner text</span>
                        <input
                            type="text"
                            value={banner.text}
                            maxLength={60}
                            placeholder="Say something dangerous…"
                            onChange={(event) => onChange({ ...banner, text: event.target.value })}
                            autoFocus
                        />
                    </label>
                    <div className="text-banner-styles" aria-label="Text banner style">
                        {STYLE_OPTIONS.map((option) => (
                            <button
                                key={option.id}
                                type="button"
                                className={`text-banner-style text-banner-style-${option.id}`}
                                aria-pressed={banner.style === option.id}
                                onClick={() => onChange({ ...banner, style: option.id })}
                            >
                                {option.label}
                            </button>
                        ))}
                    </div>
                    <label className="text-banner-position">
                        Position
                        <input
                            type="range"
                            min="18"
                            max="72"
                            value={banner.position}
                            onChange={(event) => onChange({ ...banner, position: Number(event.target.value) })}
                        />
                    </label>
                    {banner.text && (
                        <button
                            type="button"
                            className="text-banner-clear"
                            onClick={() => onChange({ ...banner, text: '' })}
                        >
                            Clear text
                        </button>
                    )}
                </div>
            )}
        </div>
    );
}
