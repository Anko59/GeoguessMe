# Geoguessme

A real-time multiplayer location guessing game inspired by Snapchat and Geoguessr.

## Features
- **Groups**: Create or join groups with friends.
- **Photo Sharing**: Snap a photo, and it's instantly shared with your group.
- **Geolocation**: Photos are automatically tagged with your location.
- **Game**: Friends have 10 seconds to view the photo, then must guess the location on a map.
- **Scoring**: Points are awarded based on accuracy.
- **Leaderboard**: See who knows their geography best.
- **Chat**: Real-time chat within the group.

## Tech Stack
- **Backend**: Go (Golang), PostgreSQL, WebSocket.
- **Frontend**: React, TypeScript, Vite, Leaflet (Maps).
- **Infrastructure**: Docker Compose.

## Getting Started

### Prerequisites
- Docker & Docker Compose
- Make (optional)

### Running the App
1.  Start the services:
    ```bash
    make up
    # OR
    docker compose up -d
    ```

2.  Open the frontend:
    [http://localhost:5173](http://localhost:5173)

3.  To stop:
    ```bash
    make down
    ```

## Development
- **Backend**: Located in `backend/`. Runs on port 8080.
- **Frontend**: Located in `frontend/`. Runs on port 5173.

## Testing
Run backend tests:
```bash
make test
```
