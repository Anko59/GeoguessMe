import { useLayoutEffect, useRef, type ChangeEventHandler } from 'react';
import './MessageInput.css';

interface MessageInputProps {
    id: string;
    value: string;
    onChange: ChangeEventHandler<HTMLTextAreaElement>;
    placeholder: string;
    maxLength: number;
    disabled?: boolean;
    required?: boolean;
    className?: string;
}

/** Shared autosizing text input for group messages and feed comments. */
export default function MessageInput({
    id,
    value,
    onChange,
    placeholder,
    maxLength,
    disabled = false,
    required = false,
    className = '',
}: MessageInputProps) {
    const inputRef = useRef<HTMLTextAreaElement>(null);

    useLayoutEffect(() => {
        const textarea = inputRef.current;
        if (!textarea) return;
        textarea.style.height = 'auto';
        textarea.style.height = `${textarea.scrollHeight}px`;
    }, [value]);

    return (
        <textarea
            id={id}
            ref={inputRef}
            rows={1}
            value={value}
            onChange={onChange}
            placeholder={placeholder}
            className={`message-input ${className}`.trim()}
            maxLength={maxLength}
            disabled={disabled}
            required={required}
        />
    );
}
