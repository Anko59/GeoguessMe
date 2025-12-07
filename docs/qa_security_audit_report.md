# GeoGuessMe - QA & Security Audit Report

**Date:** December 4, 2025  
**Auditor:** Antigravity AI Agent  
**Scope:** Full stack application (Backend: Go, Frontend: React/TypeScript)

---

## Executive Summary

This comprehensive audit identified **15 critical issues** across security, functionality, and user experience. The application has fundamental security vulnerabilities that must be addressed before production deployment, including hardcoded secrets, missing authorization checks, and inadequate file upload validation.

### Severity Breakdown
- 🔴 **Critical:** 5 issues
- 🟠 **High:** 4 issues  
- 🟡 **Medium:** 4 issues
- 🟢 **Low:** 2 issues

---

## 🔴 Critical Issues

### 1. Hardcoded JWT Secret Key
**Severity:** CRITICAL  
**Location:** [auth.go:10](file:///home/anko/geoguessme/backend/internal/auth/auth.go#L10)

**Description:**  
The JWT secret key is hardcoded as `"secret_key_change_me"` in the source code. This is a severe security vulnerability that allows anyone with access to the codebase to forge authentication tokens.

**Evidence:**
```go
var jwtKey = []byte("secret_key_change_me") // TODO: Move to env var
```

**Impact:**
- Attackers can create valid JWT tokens for any user
- Complete authentication bypass
- Unauthorized access to all protected endpoints

**Reproduction:**
1. View the source code at `backend/internal/auth/auth.go`
2. Use the exposed secret to forge tokens

**Recommendation:**
- Move JWT secret to environment variable
- Use a cryptographically strong random secret (32+ bytes)
- Rotate the secret immediately if this code is in production

---

### 2. No File Type Validation on Upload
**Severity:** CRITICAL  
**Location:** [photo.go:59-84](file:///home/anko/geoguessme/backend/handlers/photo.go#L59-L84)

**Description:**  
The photo upload endpoint accepts any file type without validation. Attackers can upload malicious files (executables, scripts, etc.) which are then served publicly.

**Evidence:**
```go
file, header, err := r.FormFile("photo")
// ... no validation of file type or content
filename := uuid.New().String() + filepath.Ext(header.Filename)
```

**Impact:**
- Arbitrary file upload vulnerability
- Potential for remote code execution
- Storage abuse
- Serving malware to users

**Reproduction:**
1. Upload a `.exe` or `.sh` file instead of an image
2. File is accepted and stored
3. File is accessible at `/uploads/<filename>`

**Recommendation:**
- Validate file MIME type (check magic bytes, not just extension)
- Limit file size ( already set to 10MB)
- Only allow specific image formats (JPEG, PNG, WebP)
- Store uploads outside web root or use object storage with proper permissions

---

### 3. Missing Authorization on Leaderboard Endpoint
**Severity:** CRITICAL  
**Location:** [main.go:30](file:///home/anko/geoguessme/backend/main.go#L30) & [group.go:129-151](file:///home/anko/geoguessme/backend/handlers/group.go#L129-L151)

**Description:**  
The `/group/leaderboard` endpoint has no authentication middleware and no authorization checks. Anyone can view any group's leaderboard without being a member or even logged in.

**Evidence:**
```go
// In main.go:
http.HandleFunc("/group/leaderboard", GetLeaderboard) // No auth currently

// In group.go:
func GetLeaderboard(w http.ResponseWriter, r *http.Request) {
    // ... no auth check
    // Auth check (omitted for brevity, but should be here)
```

**Impact:**
- Information disclosure
- Privacy violation
- Potential for reconnaissance before attacks

**Reproduction:**
1. Make a GET request to `/api/group/leaderboard?group_id=<any-group-id>`
2. Receive leaderboard data without authentication

**Recommendation:**
- Add `AuthMiddleware` to the leaderboard endpoint
- Verify user is a member of the requested group
- Return 403 Forbidden if not authorized

---

### 4. Score Calculation Algorithm Failure
**Severity:** CRITICAL (Functional)  
**Location:** [score.go:25-33](file:///home/anko/geoguessme/backend/internal/game/score.go#L25-L33) & [score_test.go:29](file:///home/anko/geoguessme/backend/internal/game/score_test.go#L29)

**Description:**  
The score calculation function fails its unit test, returning incorrect scores. For a 2km distance, it returns 4524 points instead of the expected ~1800-1900.

**Evidence:**
```
--- FAIL: TestCalculateScore (0.00s)
    score_test.go:36: Distance 2000: expected score between 1800 and 1900, got 4524
```

**Impact:**
- Unfair gameplay
- Leaderboard corruption
- Poor user experience
- Loss of trust in game mechanics

**Reproduction:**
1. Run `make test-backend`
2. Observe failing test in `geoguessme/internal/game`

**Recommendation:**
- Review the exponential decay formula
- The current formula: `5000 * e^(-distance/20000)` doesn't match test expectations
- Fix formula or adjust test expectations to match intended game design

---

### 5. No CORS Configuration
**Severity:** CRITICAL (Production)  
**Location:** [main.go](file:///home/anko/geoguessme/backend/main.go)

**Description:**  
The backend has no CORS (Cross-Origin Resource Sharing) configuration. While this works in development (proxy handles it), it will fail in production when frontend and backend are on different origins.

**Impact:**
- Application won't work in production deployment
- Browser will block all API requests
- Complete service outage

**Reproduction:**
1. Deploy frontend and backend on different domains
2. Attempt to make API calls
3. Browser blocks requests due to CORS policy

**Recommendation:**
- Implement CORS middleware
- Configure allowed origins based on environment
- Set appropriate headers: `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods`, etc.

---

## 🟠 High Severity Issues

### 6. No Logout Functionality
**Severity:** HIGH (UX/Security)  
**Location:** Frontend UI

**Description:**  
The application has no logout button or functionality. Users cannot properly end their session without manually clearing browser storage.

**Impact:**
- Users cannot log out from shared devices
- Security risk on public/shared computers
- Poor user experience

**Reproduction:**
1. Log in to the application
2. Search for logout button in all pages (Home, Groups, Group View, Settings)
3. No logout option found

**Recommendation:**
- Add logout button to settings modal or header
- Clear `localStorage` items: `token` and `user`
- Redirect to login page
- Consider implementing server-side token invalidation

---

### 7. Missing Input Validation
**Severity:** HIGH  
**Locations:** Multiple endpoints

**Description:**  
Several endpoints lack proper input validation:
- **Password Requirements:** No minimum length, complexity requirements
- **Username Validation:** No checks for special characters, length limits
- **Group Code Uniqueness:** No retry logic if collision occurs
- **Latitude/Longitude Bounds:** No validation of valid ranges

**Impact:**
- Weak passwords allow brute force attacks
- Database integrity issues
- Potential for SQL injection in future code changes
- Application crashes from invalid coordinates

**Recommendation:**
- Implement password policy (min 8 chars, complexity)
- Validate username format (alphanumeric + limited special chars)
- Add retry logic for group code generation
- Validate lat/long ranges (-90 to 90, -180 to 180)

---

### 8. WebSocket Token Exposure in URL
**Severity:** HIGH  
**Location:** [GroupView.tsx:82](file:///home/anko/geoguessme/frontend/src/pages/GroupView.tsx#L82)

**Description:**  
The JWT token is passed in the WebSocket URL query string, which may be logged by proxies, browsers, and servers.

**Evidence:**
```typescript
const wsUrl = `${protocol}//${host}/ws?group_id=${id}&token=${token}`;
```

**Impact:**
- Token exposure in server logs
- Token exposure in browser history
- Token exposure in proxy logs
- Potential token leakage

**Recommendation:**
- Use WebSocket sub-protocols for authentication
- Or send token in first WebSocket message after connection
- Avoid putting sensitive data in URLs

---

### 9. No File Size Limit Enforcement
**Severity:** HIGH  
**Location:** [photo.go:30](file:///home/anko/geoguessme/backend/handlers/photo.go#L30)

**Description:**  
While `ParseMultipartForm(10 << 20)` sets a 10MB limit in memory, there's no explicit validation that uploaded files don't exceed this size, and no user-facing error for oversized uploads.

**Impact:**
- Potential memory exhaustion
- Storage quota abuse
- Poor user experience (no clear error)

**Recommendation:**
- Explicitly check file size before processing
- Return clear error message for oversized files
- Consider lower limit for mobile bandwidth (2-5MB)

---

## 🟡 Medium Severity Issues

### 10. No Logged-In State on Landing Page
**Severity:** MEDIUM (UX)  
**Location:** [Home.tsx](file:///home/anko/geoguessme/frontend/src/pages/Home.tsx)

**Description:**  
When a logged-in user navigates to `/`, the landing page doesn't reflect their logged-in state or redirect them to `/groups`.

**Impact:**
- Confusing user experience
- No obvious way to access main app from home
- Wasted bandwidth loading unnecessary page

**Recommendation:**
- Check for token in `localStorage` on Home page load
- Redirect to `/groups` if logged in
- Or show "Continue to App" button instead of Login/Signup

---

### 11. Race Condition in WebSocket
**Severity:** MEDIUM  
**Location:** [GroupView.tsx:61-103](file:///home/anko/geoguessme/frontend/src/pages/GroupView.tsx#L61-L103)

**Description:**  
Messages from the REST API (line 71-76) and WebSocket (line 86-94) can arrive in the wrong order, potentially causing duplicate messages or missed updates.

**Impact:**
- Messages may appear out of order
- Potential duplicate messages
- Inconsistent UI state

**Recommendation:**
- Use message IDs to deduplicate
- Implement proper message ordering/sequencing
- Consider using only WebSocket for message delivery

---

### 12. Sensitive Error Messages
**Severity:** MEDIUM  
**Location:** Multiple handlers

**Description:**  
Several endpoints return detailed error messages that expose internal implementation details.

**Examples:**
- `"Database error"` - reveals database usage
- `fmt.Printf` statements log sensitive info to stdout
- Stack traces may leak in development mode

**Impact:**
- Information disclosure
- Aids attackers in reconnaissance
- Exposes technology stack

**Recommendation:**
- Return generic error messages to clients
- Log detailed errors server-side only
- Implement error codes instead of messages
- Remove debug `fmt.Printf` statements

---

### 13. No Rate Limiting
**Severity:** MEDIUM  
**Locations:** All endpoints

**Description:**  
No rate limiting on any endpoint, allowing unlimited requests.

**Impact:**
- Brute force attacks on login
- DoS attacks
- Resource exhaustion
- Spam prevention issues

**Recommendation:**
- Implement rate limiting middleware
- Different limits for different endpoints
- Consider IP-based and token-based limiting
- Special care for expensive operations (file uploads, authentication)

---

## 🟢 Low Severity Issues

### 14. Random Number Generator Not Seeded
**Severity:** LOW  
**Location:** [auth.go:84](file:///home/anko/geoguessme/backend/handlers/auth.go#L84) & [group.go:23-29](file:///home/anko/geoguessme/backend/handlers/group.go#L23-L29)

**Description:**  
Using `math/rand` without seeding for avatar selection and group code generation. While UUIDs provide randomness for group codes, avatar selection may be predictable.

**Impact:**
- Potentially predictable avatar assignments
- Group codes currently protected by UUID

**Recommendation:**
- Use `crypto/rand` for cryptographic randomness
- Or seed `math/rand` in `main()` with `rand.Seed(time.Now().UnixNano())`

---

### 15. Missing HTTP Security Headers
**Severity:** LOW  
**Location:** [main.go](file:///home/anko/geoguessme/backend/main.go)

**Description:**  
No security headers configured (X-Frame-Options, X-Content-Type-Options, etc.)

**Impact:**
- Clickjacking vulnerability
- MIME sniffing attacks
- XSS in some scenarios

**Recommendation:**
- Add security headers middleware
- Set: `X-Frame-Options: DENY`
- Set: `X-Content-Type-Options: nosniff`
- Set: `X-XSS-Protection: 1; mode=block`
- Consider Content Security Policy (CSP)

---

## Additional Observations

### ✅ Good Practices Found
1. **Parameterized SQL Queries** - All database queries use parameterized statements, preventing SQL injection
2. **Password Hashing** - Using bcrypt with appropriate cost factor (14)
3. **UUID Usage** - Proper use of UUIDs for entity IDs
4. **Health Check Endpoint** - Implemented for monitoring
5. **Docker Setup** - Clean containerization with health checks

### Test Coverage Analysis
- **Backend:** Minimal test coverage, only 2 test files found
  - `handlers/auth_test.go` - Basic auth tests
  - `internal/game/score_test.go` - Failing tests
- **Frontend:** Single test file (`App.test.tsx`) - passes but minimal coverage
- **Integration Tests:** Directory exists but coverage unknown
- **Recommendation:** Increase test coverage to at least 60-70%

---

## Prioritized Remediation Plan

### Phase 1: Immediate (Before Any Production Use)
1. Fix hardcoded JWT secret (#1)
2. Add file type validation (#2)
3. Fix leaderboard authorization (#3)
4. Implement CORS configuration (#5)

### Phase 2: Critical Fixes (Within 1 Week)
5. Fix score calculation bug (#4)
6. Add input validation (#7)
7. Fix WebSocket token exposure (#8)
8. Add rate limiting (#13)

### Phase 3: UX & Security Hardening (Within 2 Weeks)
9. Add logout functionality (#6)
10. Improve error handling (#12)
11. Add security headers (#15)
12. Fix file size validation (#9)

### Phase 4: Polish & Long-term (Within 1 Month)
13. Fix logged-in state handling (#10)
14. Address WebSocket race conditions (#11)
15. Improve random number generation (#14)
16. Increase test coverage

---

## Conclusion

The GeoGuessMe application has a solid foundation but requires significant security hardening before production deployment. The most critical issues involve authentication, authorization, and file upload security. Addressing the Phase 1 issues is **mandatory** before any public release.

**Risk Assessment:** Current state is **NOT PRODUCTION READY**

**Estimated Effort to Fix Critical Issues:** 2-3 days of development work

---

## Testing Artifacts

### Manual Testing Recording
The manual testing session recording is available here:  
![Manual Testing Session](file:///home/anko/.gemini/antigravity/brain/5201fb04-54ea-441f-9eff-76f7f88a8eea/app_manual_testing_1764887000629.webp)

### Test Results
```
Backend Tests:
✓ handlers (passed)
✗ internal/game (failed - score calculation)
✓ integration_test (passed - cached)

Frontend Tests:
✓ All tests passed (1/1)
```
