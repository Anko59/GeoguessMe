import http from 'k6/http';
import ws from 'k6/ws';
import { check } from 'k6';
import { Rate } from 'k6/metrics';

const websocketFailures = new Rate('websocket_delivery_failures');

export const options = {
    vus: Number(__ENV.VUS || 5),
    duration: __ENV.DURATION || '30s',
    thresholds: {
        http_req_failed: ['rate<0.01'],
        http_req_duration: ['p(95)<500'],
        websocket_delivery_failures: ['rate==0'],
    },
};

export function setup() {
    const suffix = `${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
    const response = http.post(
        `${__ENV.BASE_URL}/api/v1/auth/signup`,
        JSON.stringify({
            username: `load_${suffix}`,
            email: `load_${suffix}@test.geoguessme`,
            password: 'LoadPass123',
        }),
        { headers: { 'Content-Type': 'application/json' } },
    );
    check(response, { 'setup signup succeeds': (value) => value.status === 200 });
    const body = response.json();
    const access = body.access_token;
    const group = http.post(`${__ENV.BASE_URL}/api/v1/group/create`, JSON.stringify({ name: `LoadGroup_${suffix}` }), {
        headers: { Authorization: `Bearer ${access}`, 'Content-Type': 'application/json' },
    });
    check(group, { 'setup group succeeds': (value) => value.status === 201 });
    return {
        username: `load_${suffix}`,
        password: 'LoadPass123',
        access,
        groupID: group.json('id'),
    };
}

export default function (credentials) {
    const login = http.post(
        `${__ENV.BASE_URL}/api/v1/auth/login`,
        JSON.stringify({
            username: credentials.username,
            password: credentials.password,
        }),
        { headers: { 'Content-Type': 'application/json' } },
    );
    check(login, { 'login succeeds': (value) => value.status === 200 });
    const access = login.json('access_token') || credentials.access;
    const refreshCookie = login.cookies.refresh_token?.[0]?.value;
    const params = { headers: { Authorization: `Bearer ${access}` } };
    check(http.get(`${__ENV.BASE_URL}/api/v1/user/groups`, params), {
        'group read succeeds': (value) => value.status === 200,
    });
    check(http.get(`${__ENV.BASE_URL}/api/v1/group/messages?group_id=${credentials.groupID}`, params), {
        'message history read succeeds': (value) => value.status === 200,
    });
    const refresh = http.post(`${__ENV.BASE_URL}/api/v1/auth/refresh`, null, {
        cookies: { refresh_token: refreshCookie },
    });
    check(refresh, { 'refresh succeeds': (value) => value.status === 200 });

    const ticket = http.post(`${__ENV.BASE_URL}/api/v1/ws/ticket?group_id=${credentials.groupID}`, null, params);
    check(ticket, { 'websocket ticket succeeds': (value) => value.status === 201 });
    if (ticket.status !== 201) {
        websocketFailures.add(1);
        return;
    }
    const wsBase = __ENV.BASE_URL.replace(/^http/, 'ws');
    const ticketValue = ticket.json('ticket');
    let delivered = false;
    const socketResponse = ws.connect(
        `${wsBase}/api/v1/ws?group_id=${credentials.groupID}&ticket=${ticketValue}`,
        {},
        (socket) => {
            socket.on('open', () => {
                socket.send(JSON.stringify({ content: `load-${__VU}-${__ITER}` }));
                socket.setTimeout(() => socket.close(), 2000);
            });
            socket.on('message', (message) => {
                try {
                    const parsed = JSON.parse(message);
                    if (parsed.kind === 'text') {
                        delivered = true;
                    }
                } catch {
                    // A malformed server frame is treated as a delivery failure.
                }
            });
            socket.on('error', () => socket.close());
        },
    );
    websocketFailures.add(socketResponse.status !== 101 || !delivered);
}
