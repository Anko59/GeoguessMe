import { useEffect, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import type { GroupChallenge, GroupChallengesPage } from '../../types';

export function useGroupChallenges(groupID: string) {
    const [revision, setRevision] = useState(0);
    const [state, setState] = useState({
        groupID: '',
        revision: -1,
        items: [] as GroupChallenge[],
        loading: true,
        error: '',
    });
    useEffect(() => {
        const controller = new AbortController();
        const load = async () => {
            let items: GroupChallenge[] = [];
            let cursor: string | undefined;
            const cursors = new Set<string>();
            try {
                do {
                    const { data } = await api.get<GroupChallengesPage>('/group/challenges', {
                        params: { group_id: groupID, cursor },
                        signal: controller.signal,
                        timeout: 15_000,
                    });
                    if (controller.signal.aborted) return;
                    items = [...items, ...data.items];
                    cursor = data.next_cursor;
                    if (cursor && cursors.has(cursor))
                        throw new Error('Unable to load the remaining challenges. Please refresh.');
                    if (cursor) cursors.add(cursor);
                    setState({ groupID, revision, items, loading: Boolean(cursor), error: '' });
                } while (cursor);
            } catch (error) {
                if (!controller.signal.aborted) {
                    // Clear even partial data on access loss; no private data cache.
                    setState({
                        groupID,
                        revision,
                        items: [],
                        loading: false,
                        error: getAPIErrorMessage(error, 'Unable to load group challenges'),
                    });
                }
            }
        };
        void load();
        return () => controller.abort();
    }, [groupID, revision]);
    const current = state.groupID === groupID && state.revision === revision;
    return {
        items: current ? state.items : [],
        loading: !current || state.loading,
        error: current ? state.error : '',
        refresh: () => setRevision((value) => value + 1),
    };
}
