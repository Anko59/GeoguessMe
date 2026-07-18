import http from 'k6/http';
import ws from 'k6/ws';
import { check } from 'k6';
import { Rate, Counter } from 'k6/metrics';

const websocketFailures = new Rate('websocket_delivery_failures');
const rateLimitEnforced = new Counter('rate_limit_enforced');

const VUS = Number(__ENV.VUS || 5);

export const options = {
    vus: VUS,
    duration: __ENV.DURATION || '30s',
    thresholds: {
        http_req_failed: ['rate<0.01'],
        http_req_duration: ['p(95)<500'],
        websocket_delivery_failures: ['rate==0'],
        rate_limit_enforced: ['count>=1'],
    },
};

export function setup() {
    // Each VU gets a distinct identity so the per-identity rate limiter
    // (RATE_LIMIT_REQUESTS=3, RATE_LIMIT_WINDOW=10s in the test stack)
    // never fires on legitimate distinct clients that log in once.
    const suffix = `${Date.now()}_${Math.random().toString(36).slice(2, 6)}`;
    const users = [];
    for (let i = 0; i < VUS; i++) {
        const username = `load_${suffix}_${i}`;
        const signup = http.post(
            `${__ENV.BASE_URL}/api/v1/auth/signup`,
            JSON.stringify({ username, email: `${username}@test.geoguessme`, password: 'LoadPass123' }),
            { headers: { 'Content-Type': 'application/json' } },
        );
        check(signup, { [`signup vu${i}`]: (v) => v.status === 200 });

        const login = http.post(
            `${__ENV.BASE_URL}/api/v1/auth/login`,
            JSON.stringify({ username, password: 'LoadPass123' }),
            { headers: { 'Content-Type': 'application/json' } },
        );
        check(login, { [`login vu${i}`]: (v) => v.status === 200 });

        const access = login.json('access_token');
        const refreshCookie = login.cookies.refresh_token?.[0]?.value;

        const group = http.post(
            `${__ENV.BASE_URL}/api/v1/group/create`,
            JSON.stringify({ name: `ldgrp_${suffix}_${i}` }),
            { headers: { Authorization: `Bearer ${access}`, 'Content-Type': 'application/json' } },
        );
        check(group, { [`group vu${i}`]: (v) => v.status === 201 });

        users.push({ username, password: 'LoadPass123', access, refreshCookie, groupID: group.json('id') });
    }

    // Verify that the identity rate limiter fires when a single identity
    // exceeds the configured quota.  The setup login already consumed one
    // slot; three more login calls from the same identity push past the
    // per-window limit (3 req / 10 s), confirming the security control is
    // active without weakening it.
    const probe = users[0];
    for (let i = 0; i < 3; i++) {
        const r = http.post(
            `${__ENV.BASE_URL}/api/v1/auth/login`,
            JSON.stringify({ username: probe.username, password: probe.password }),
            { headers: { 'Content-Type': 'application/json' } },
        );
        if (r.status === 429) rateLimitEnforced.add(1);
    }

    return { users };
}

export default function (data) {
    // Each VU stays within its own identity budget.  No per-iteration login
    // is needed because setup pre-authenticated every identity once —
    // exactly what distinct real-world clients do.
    const user = data.users[(__VU - 1) % data.users.length];

    // Use the current access token (updated by refresh below).
    const params = { headers: { Authorization: `Bearer ${user.access}` } };

    check(http.get(`${__ENV.BASE_URL}/api/v1/user/groups`, params), {
        'group read succeeds': (v) => v.status === 200,
    });
    check(http.get(`${__ENV.BASE_URL}/api/v1/group/messages?group_id=${user.groupID}`, params), {
        'message history read succeeds': (v) => v.status === 200,
    });

    // Refresh is not subject to identity rate limiting.  Each call rotates
    // the session, so capture the new cookie for the next iteration.
    const refresh = http.post(`${__ENV.BASE_URL}/api/v1/auth/refresh`, null, {
        cookies: { refresh_token: user.refreshCookie },
    });
    check(refresh, { 'refresh succeeds': (v) => v.status === 200 });
    if (refresh.status === 200) {
        user.access = refresh.json('access_token');
        const nextCookie = refresh.cookies.refresh_token?.[0]?.value;
        if (nextCookie) user.refreshCookie = nextCookie;
    }

    const ticket = http.post(`${__ENV.BASE_URL}/api/v1/ws/ticket?group_id=${user.groupID}`, null, params);
    check(ticket, { 'websocket ticket succeeds': (v) => v.status === 201 });
    if (ticket.status !== 201) {
        websocketFailures.add(1);
        return;
    }

    const wsBase = __ENV.BASE_URL.replace(/^http/, 'ws');
    const ticketValue = ticket.json('ticket');
    let delivered = false;
    const socketResponse = ws.connect(
        `${wsBase}/api/v1/ws?group_id=${user.groupID}&ticket=${ticketValue}`,
        {},
        (socket) => {
            socket.on('open', () => {
                socket.send(JSON.stringify({ content: `load-${__VU}-${__ITER}` }));
                socket.setTimeout(() => socket.close(), 2000);
            });
            socket.on('message', (msg) => {
                try {
                    if (JSON.parse(msg).kind === 'text') delivered = true;
                } catch {
                    // A malformed server frame is treated as a delivery failure.
                }
            });
            socket.on('error', () => socket.close());
        },
    );
    websocketFailures.add(socketResponse.status !== 101 || !delivered);
}
