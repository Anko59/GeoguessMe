# GeoGuessMe - Testing Guide

## Overview

This document outlines comprehensive testing procedures for the GeoGuessMe application, including automated tests, manual testing procedures, and security verification.

---

## Automated Testing

### Backend Tests

Run all backend tests:
```bash
make test-backend
```

This runs tests for:
- Authentication handlers
- Game logic (score calculation, distance calculation)
- Integration tests

**Expected Output:**
All tests should pass with no failures.

### Frontend Tests

Run frontend tests:
```bash
make test-frontend
```

Or directly:
```bash
cd frontend && npm test
```

**Expected Output:**
All React component tests should pass.

### Running Specific Tests

```bash
# Run only game tests
cd backend && go test ./internal/game

# Run only handler tests
cd backend && go test ./handlers

# Run with verbose output
cd backend && go test -v ./...
```

---

## Manual Testing Procedures

### 1. Authentication & Authorization

#### Test: User Signup with Password Validation
**Steps:**
1. Navigate to `/signup`
2. Try to create account with username: `test` (too short)
   - **Expected:** Error "username: must be at least 3 characters"
3. Try with username: `validuser` and password: `weak`
   - **Expected:** Error "password: must be at least 8 characters"
4. Try with password: `password123` (no uppercase)
   - **Expected:** Error "password: must contain uppercase, lowercase, and numbers"
5. Create account with username: `validuser` and password: `TestPass123`
   - **Expected:** Success, redirected to groups page

#### Test: Login Security
**Steps:**
1. Navigate to `/login`
2. Try logging in with wrong credentials
   - **Expected:** Generic error "Authentication failed" (not revealing if username or password is wrong)
3. Login with correct credentials
   - **Expected:** Success, redirected to groups

#### Test: Logout Functionality
**Steps:**
1. While logged in, navigate to any group
2. Click settings icon
3. Scroll down and click "Logout" button
   - **Expected:** Redirected to home page, localStorage cleared
4. Try navigating to `/groups`
   - **Expected:** Redirected to login page

#### Test: Logged-In State on Home Page
**Steps:**
1. While logged out, navigate to `/`
   - **Expected:** See login and signup buttons
2. Log in to the application
3. Navigate to `/` while logged in
   - **Expected:** Automatically redirected to `/groups`

---

### 2. File Upload Security

#### Test: File Type Validation
**Steps:**
1. Join or create a group
2. Navigate to camera tab
3. Attempt to upload a non-image file (create a text file and rename it to `.jpg`)
   - **Expected:** Error "invalid file type: only JPEG, PNG, and WebP images are allowed"
4. Upload an actual image file
   - **Expected:** Success, photo uploaded

#### Test: File Size Validation
**Steps:**
1. Try to upload an image larger than 5MB
   - **Expected:** Error about file size exceeding maximum
2. Upload a normal-sized photo
   - **Expected:** Success

#### Test: Coordinate Validation
**Steps:**
1. Using browser developer tools, inspect the network request for photo upload
2. Modify the `lat` parameter to `100` (invalid latitude)
   - **Expected:** Server returns error "latitude: must be between -90 and 90"
3. Modify the `long` parameter to `200` (invalid longitude)
   - **Expected:** Server returns error "longitude: must be between -180 and 180"

---

### 3. Authorization & Access Control

#### Test: Leaderboard Authorization
**Steps:**
1. Create two users: UserA and UserB
2. UserA creates GroupX
3. UserB creates GroupY
4. As UserB, try to access `/api/group/leaderboard?group_id=<GroupX_ID>` (UserB is not a member)
   - **Expected:** 403 Forbidden error
5. As UserA, access the same endpoint
   - **Expected:** Success, leaderboard data returned

#### Test: Group Member-Only Features
**Steps:**
1. Try accessing group details for a group you're not a member of
   - **Expected:** Appropriate authorization error
2. Join the group and try again
   - **Expected:** Success

---

### 4. Rate Limiting

#### Test: Login Rate Limiting
**Steps:**
1. Open browser developer console
2. Run the following JavaScript to make rapid login attempts:
```javascript
for (let i = 0; i < 15; i++) {
  fetch('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'test', password: 'test' })
  }).then(r => console.log(i, r.status));
}
```
3. **Expected:** First 10 requests get through, subsequent requests return 429 (Too Many Requests)
4. Wait 1 minute and try again
   - **Expected:** Rate limit reset, requests succeed again

---

### 5. Input Validation

#### Test: Group Code Validation
**Steps:**
1. Try joining a group with code: `abc` (too short)
   - **Expected:** Error "code: must be exactly 6 characters"
2. Try with code: `abcdef` (lowercase)
   - **Expected:** Error "code: must contain only uppercase letters and numbers"
3. Try with valid code: `ABC123`
   - **Expected:** Success if group exists

#### Test: Group Name Validation
**Steps:**
1. Create group with empty name: ``
   - **Expected:** Error "name: is required"
2. Create group with very long name (>100 chars)
   - **Expected:** Error "name: must be at most 100 characters"

---

### 6. CORS & Security Headers

#### Test: CORS Headers
**Steps:**
1. Open browser developer tools → Network tab
2. Make any API request
3. Inspect response headers
   - **Expected:** See headers:
     - `Access-Control-Allow-Origin: <your-origin>`
     - `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS`
     - `Access-Control-Allow-Headers: Content-Type, Authorization`

#### Test: Security Headers
**Steps:**
1. Inspect any response headers
   - **Expected:** See:
     - `X-Frame-Options: DENY`
     - `X-Content-Type-Options: nosniff`
     - `X-XSS-Protection: 1; mode=block`
     - `Content-Security-Policy: ...`

---

## Security Testing Checklist

### Authentication & Session Management
- [ ] JWT secret is loaded from environment variable
- [ ] Tokens expire after 24 hours
- [ ] Logout clears all client-side tokens
- [ ] Password requirements enforced (8+ chars, uppercase, lowercase, number)
- [ ] Passwords hashed with bcrypt
- [ ] Generic error messages don't reveal user existence

### Authorization
- [ ] Leaderboard requires group membership
- [ ] Users can only access groups they're members of
- [ ] File uploads check user authentication
- [ ] WebSocket requires authentication

### Input Validation
- [ ] Username validation (3-30 chars, alphanumeric + _ -)
- [ ] Password validation (8+ chars, complexity)
- [ ] Coordinates validation (-90 to 90, -180 to 180)
- [ ] Group code validation (6 chars, uppercase alphanumeric)
- [ ] Group name validation (1-100 chars)

### File Upload Security
- [ ] File type validation by magic bytes (not just extension)
- [ ] File size limited to 5MB
- [ ] Only JPEG, PNG, WebP allowed
- [ ] Filenames sanitized
- [ ] Files served with proper content type

### Rate Limiting
- [ ] Login endpoint limited to 10 requests/minute
- [ ] Signup endpoint limited to 10 requests/minute
- [ ] Rate limits enforced per IP address

### Security Headers & CORS
- [ ] CORS configured with allowed origins from env
- [ ] X-Frame-Options set to DENY
- [ ] X-Content-Type-Options set to nosniff
- [ ] Content-Security-Policy configured
- [ ] Referrer-Policy configured

---

## Pre-Deployment Verification

Before deploying to production, verify:

### Environment Configuration
```bash
# .env or environment variables must include:
JWT_SECRET=<strong-32+-char-random-string>
ALLOWED_ORIGINS=https://yourdomain.com
DATABASE_URL=<production-db-url>
```

### Generate Strong JWT Secret
```bash
openssl rand -base64 32
```

### Database Migrations
```bash
# Ensure schema is initialized
# Database migrations run automatically on startup
```

### Docker Compose Check
```bash
# Verify environment variables are set
docker-compose config

# Start services
docker-compose up -d

# Check health
docker-compose ps
curl http://localhost:8080/health
```

### Final Test Run
```bash
# Run all tests
make test

# Check logs for errors
docker-compose logs backend
docker-compose logs frontend
```

---

## Continuous Monitoring

### Health Check
```bash
curl http://localhost:8080/health
# Expected: 200 OK
```

### Log Monitoring
```bash
# Watch backend logs
docker-compose logs -f backend

# Watch for errors
docker-compose logs backend | grep -i error
```

---

## Troubleshooting

### Tests Failing

**Issue:** JWT_SECRET not set error
**Solution:** Tests automatically use a test secret. If still failing, check that test detection logic is working.

**Issue:** Database connection errors
**Solution:** Ensure PostgreSQL container is running:
```bash
docker-compose up -d db
```

### Runtime Issues

**Issue:** CORS errors in browser
**Solution:** Verify `ALLOWED_ORIGINS` environment variable includes your frontend origin.

**Issue:** File upload fails
**Solution:** Check that `/uploads` directory exists and has write permissions.

**Issue:** Rate limiting too strict
**Solution:** Adjust limits in `main.go`:
```go
authRateLimit := middleware.RateLimit(20, time.Minute) // Increase from 10 to 20
```

---

## Test Coverage Goals

- **Backend:** 60%+ overall, 80%+ for critical paths (auth, file upload, authorization)
- **Frontend:** 50%+ overall, focus on critical user flows
- **Integration:** Cover all major user journeys end-to-end

---

## Future Testing Improvements

1. **Automated Security Scanning**
   - Integrate OWASP ZAP or similar
   - Regular dependency vulnerability scans

2. **Performance Testing**
   - Load testing with k6 or similar
   - Database query optimization

3. **E2E Testing**
   - Playwright or Cypress for full user flows
   - Cross-browser testing

4. **Monitoring & Alerts**
   - Application Performance Monitoring (APM)
   - Error tracking (Sentry, etc.)
   - Uptime monitoring
