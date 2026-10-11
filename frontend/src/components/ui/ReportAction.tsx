import { useEffect, useRef, useState, type FormEvent } from 'react';
import { getAPIErrorMessage, moderationAPI } from '../../api';
import type { components } from '../../types/openapi.generated';
import './ReportAction.css';

type ReportRequest = components['schemas']['ReportRequest'];

interface ReportActionProps {
    kind: 'messages' | 'users';
    targetID: string;
}

/** Inline notice form; no private details are stored in client state after success. */
export default function ReportAction({ kind, targetID }: ReportActionProps) {
    const [open, setOpen] = useState(false);
    const [reason, setReason] = useState<ReportRequest['reason']>('other');
    const [details, setDetails] = useState('');
    const [pending, setPending] = useState(false);
    const [receipt, setReceipt] = useState('');
    const [error, setError] = useState('');
    const controllerRef = useRef<AbortController | null>(null);
    useEffect(() => () => controllerRef.current?.abort(), []);

    async function submit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (pending) return;
        const controller = new AbortController();
        controllerRef.current = controller;
        setPending(true);
        setError('');
        try {
            const response = await moderationAPI.report(
                kind,
                targetID,
                { reason, details } satisfies ReportRequest,
                controller.signal,
            );
            if (controller.signal.aborted) return;
            setReceipt(response.id);
            setDetails('');
            setOpen(false);
        } catch (requestError: unknown) {
            if (!controller.signal.aborted) {
                setError(getAPIErrorMessage(requestError, 'Unable to submit your report. Please try again.'));
            }
        } finally {
            if (!controller.signal.aborted) setPending(false);
            controllerRef.current = null;
        }
    }

    if (receipt)
        return (
            <p className="report-action-status" role="status">
                Report received. Reference: {receipt}
            </p>
        );

    return (
        <div className="report-action">
            <button
                className="btn btn-secondary"
                type="button"
                aria-expanded={open}
                onClick={() => setOpen((value) => !value)}
            >
                Report {kind === 'users' ? 'player' : 'message'}
            </button>
            {open && (
                <form onSubmit={(event) => void submit(event)} aria-label="Submit content report">
                    <label>
                        Reason
                        <select
                            value={reason}
                            onChange={(event) => setReason(event.target.value as ReportRequest['reason'])}
                        >
                            <option value="illegal_content">Illegal content</option>
                            <option value="harassment">Harassment</option>
                            <option value="sexual_content">Sexual content</option>
                            <option value="other">Other</option>
                        </select>
                    </label>
                    <label>
                        Details {reason === 'illegal_content' ? '(required)' : '(optional)'}
                        <textarea
                            value={details}
                            maxLength={2000}
                            required={reason === 'illegal_content'}
                            onChange={(event) => setDetails(event.target.value)}
                        />
                    </label>
                    {error && <p role="alert">{error}</p>}
                    <button className="btn btn-primary" type="submit" disabled={pending}>
                        {pending ? 'Submitting…' : 'Submit report'}
                    </button>
                </form>
            )}
        </div>
    );
}
