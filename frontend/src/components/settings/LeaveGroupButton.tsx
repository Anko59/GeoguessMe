import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { getAPIErrorMessage, groupsAPI } from '../../api';
import Icon from '../ui/Icon';

export default function LeaveGroupButton({ groupId }: { groupId: string }) {
    const navigate = useNavigate();
    const [leaving, setLeaving] = useState(false);
    const [error, setError] = useState('');
    const pending = useRef(false);
    const active = useRef(true);

    useEffect(() => {
        active.current = true;
        return () => {
            active.current = false;
        };
    }, []);

    const leave = async () => {
        if (pending.current) return;
        pending.current = true;
        setLeaving(true);
        setError('');
        try {
            await groupsAPI.leave(groupId);
            if (active.current) navigate('/groups', { replace: true });
        } catch (requestError: unknown) {
            if (active.current) setError(getAPIErrorMessage(requestError, 'Unable to leave group. Try again.'));
        } finally {
            pending.current = false;
            if (active.current) setLeaving(false);
        }
    };

    return (
        <>
            <button type="button" className="leave-group-btn" disabled={leaving} onClick={() => void leave()}>
                <Icon name="logout" />
                {leaving ? 'Leaving…' : 'Leave the group'}
            </button>
            {error && <p role="alert">{error}</p>}
        </>
    );
}
