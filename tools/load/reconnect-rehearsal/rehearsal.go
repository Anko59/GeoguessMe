package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// main executes a 7-phase deterministic reconnect/load rehearsal.
func main() {
	flag.Parse()
	fmt.Printf("reconnect-rehearsal base=%s\n", *baseURL)

	concurrent := 6 // deterministic number of concurrent clients
	suffix := randomSuffix()
	res := &result{}

	// 1. Sign up participants and create group ---------------------------------
	fmt.Printf("[1/7] signup %d participants + create group ...\n", concurrent)
	creds := make([]credentials, concurrent)
	var wg sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			u := fmt.Sprintf("%s_%d", suffix, idx)
			c := credentials{Username: u, Password: "ReconnPass123!"}
			resp, body, err := doJSON(http.MethodPost, "/api/v1/auth/signup",
				map[string]string{"username": u, "email": u + "@test.geoguessme", "password": c.Password},
				"", nil)
			if err != nil || resp.StatusCode != http.StatusOK {
				fmt.Fprintf(os.Stderr, "signup %d failed: %v %s\n", idx, err, string(body))
				res.addError()
				return
			}
			var lr loginResponse
			if err := json.Unmarshal(body, &lr); err != nil {
				res.addError()
				return
			}
			c.Access = lr.AccessToken
			for _, ck := range resp.Cookies() {
				if ck.Name == "refresh_token" {
					c.Refresh = ck.Value
				}
			}
			creds[idx] = c
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("signup phase failed", res)
	}

	// Create group with first participant.
	group := mustVal(createGroup(creds[0].Access, "ReconnGrp_"+suffix))
	for i := 1; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := joinGroup(creds[idx].Access, group.Code); err != nil {
				fmt.Fprintf(os.Stderr, "join %d failed: %v\n", idx, err)
				res.addError()
			}
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("group join phase failed", res)
	}

	// 2. Connect all via WebSocket, send round 1 -------------------------------
	fmt.Printf("[2/7] connect %d clients + send round 1 ...\n", concurrent)
	conns, _, r1Duration := runRound1(concurrent, creds, group, res)
	_ = r1Duration // used in evidence report later

	// 3. Disconnect all clients -------------------------------------------------
	fmt.Printf("[3/7] disconnect all clients ...\n")
	for _, c := range conns {
		_ = c.Close()
	}
	conns = nil

	// 4. While everyone is disconnected, send messages from a fresh anchor -----
	fmt.Printf("[4/7] send catch-up messages while all are disconnected ...\n")
	preCursor, anchor := runDisconnectedSend(suffix, concurrent, group, res)

	// 5. Reconnect with renewed tickets -----------------------------------------
	fmt.Printf("[5/7] reconnect with renewed tickets ...\n")
	conns2, received2 := runReconnect(concurrent, creds, group, res)

	// 6. Catch up via cursor ---------------------------------------------------
	fmt.Printf("[6/7] cursor catch-up ...\n")
	runCatchUp(concurrent, creds, group, preCursor, received2, res)

	// 7. Live delivery on reconnected sockets ----------------------------------
	fmt.Printf("[7/7] live delivery on reconnected sockets ...\n")
	runLiveDelivery(concurrent, conns2, anchor, group, res)

	// Close reconnected connections.
	for _, c := range conns2 {
		_ = c.Close()
	}

	// ------- evidence report -------
	printEvidence(res, r1Duration)

	if atomic.LoadInt32(&res.Errors) > 0 {
		os.Exit(1)
	}
}

// ------- phase functions -------

func runRound1(concurrent int, creds []credentials, group groupRef, res *result) (
	[]*websocket.Conn, []map[string]bool, time.Duration,
) {
	conns := make([]*websocket.Conn, concurrent)
	received := make([]map[string]bool, concurrent)
	for i := range received {
		received[i] = map[string]bool{}
	}
	var connMu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ticket := mustVal(getWSTicket(creds[idx].Access, group.ID))
			conn, err := dialWS(group.ID, ticket)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ws dial %d failed: %v\n", idx, err)
				res.addError()
				return
			}
			connMu.Lock()
			conns[idx] = conn
			connMu.Unlock()
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("ws connect phase failed", res)
	}

	// A successful WebSocket handshake can precede the hub's registration
	// handoff. Complete a persisted readiness barrier before sending the
	// measured round so no early broadcast is lost by a still-registering peer.
	ready := make([]string, concurrent)
	for i := range ready {
		ready[i] = fmt.Sprintf("r1-ready-%d-%s", i, randomSuffix())
	}
	for i := range conns {
		if err := conns[i].WriteJSON(map[string]string{"content": ready[i]}); err != nil {
			fmt.Fprintf(os.Stderr, "write readiness %d failed: %v\n", i, err)
			res.addError()
		}
	}
	if !waitForReadiness(creds[0].Access, group.ID, ready) {
		res.addError()
		fail("ws readiness barrier failed", res)
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	conns = make([]*websocket.Conn, concurrent)
	for i := range conns {
		ticket := mustVal(getWSTicket(creds[i].Access, group.ID))
		conns[i] = mustVal(dialWS(group.ID, ticket))
	}

	// Each client sends one message, then reads N deliveries.
	tStart := time.Now()
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			content := fmt.Sprintf("r1-c%d-%s", idx, randomSuffix())
			sendStart := time.Now()
			if err := conns[idx].WriteJSON(map[string]string{"content": content}); err != nil {
				fmt.Fprintf(os.Stderr, "write r1 %d failed: %v\n", idx, err)
				res.addError()
				return
			}
			res.addSent()
			got := 0
			deadline := time.Now().Add(20 * time.Second)
			firstLatency := int64(0)
			for got < concurrent {
				_ = conns[idx].SetReadDeadline(deadline)
				_, payload, err := conns[idx].ReadMessage()
				if err != nil {
					break
				}
				var msg wsMessage
				if err := json.Unmarshal(payload, &msg); err != nil || msg.Kind != "text" {
					continue
				}
				res.incRecv()
				if _, seen := received[idx][msg.Content]; seen {
					fmt.Fprintf(os.Stderr, "duplicate '%s' on client %d\n", msg.Content, idx)
					res.addError()
				} else {
					received[idx][msg.Content] = true
				}
				if msg.Content == content && firstLatency == 0 {
					firstLatency = time.Since(sendStart).Microseconds()
					res.addLatency(firstLatency)
				}
				got++
			}
			if got != concurrent {
				fmt.Fprintf(os.Stderr, "client %d r1 got %d/%d messages\n", idx, got, concurrent)
				res.addError()
			}
		}(i)
	}
	wg.Wait()
	r1Duration := time.Since(tStart)
	fmt.Printf("  round 1: %d msgs sent, %d received in %v\n",
		atomic.LoadInt32(&res.Sent), atomic.LoadInt32(&res.Received), r1Duration)

	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("round 1 failed", res)
	}

	for i := 0; i < concurrent; i++ {
		if len(received[i]) != concurrent {
			fmt.Fprintf(os.Stderr, "client %d received %d unique msgs, want %d\n",
				i, len(received[i]), concurrent)
			res.addError()
		}
	}
	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("round 1 exact-once check failed", res)
	}
	return conns, received, r1Duration
}

func anchorFromSuffix(suffix string) credentials {
	return mustVal(signup(
		fmt.Sprintf("anchor_%s", suffix),
		fmt.Sprintf("anchor_%s@test.geoguessme", suffix),
		"ReconnPass123!",
	))
}

func runDisconnectedSend(suffix string, concurrent int, group groupRef, res *result) (string, credentials) {
	anchor := anchorFromSuffix(suffix)
	mustOK(joinGroup(anchor.Access, group.Code))
	anchorTicket := mustVal(getWSTicket(anchor.Access, group.ID))
	anchorConn := mustVal(dialWS(group.ID, anchorTicket))
	preCursor := mustVal(getLatestCursor(anchor.Access, group.ID))

	for i := 0; i < concurrent; i++ {
		content := fmt.Sprintf("missed-%d-%s", i, randomSuffix())
		if err := anchorConn.WriteJSON(map[string]string{"content": content}); err != nil {
			res.addError()
		}
		res.addSent()
	}
	// Read back own messages to confirm persistence.
	for i := 0; i < concurrent; i++ {
		_ = anchorConn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, _, err := anchorConn.ReadMessage()
		if err != nil {
			fmt.Fprintf(os.Stderr, "anchor readback %d failed: %v\n", i, err)
			res.addError()
			break
		}
		res.incRecv()
	}
	anchorConn.Close()

	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("catch-up message phase failed", res)
	}
	return preCursor, anchor
}

func runReconnect(concurrent int, creds []credentials, group groupRef, res *result) (
	[]*websocket.Conn, []map[string]bool,
) {
	conns2 := make([]*websocket.Conn, concurrent)
	received2 := make([]map[string]bool, concurrent)
	for i := range received2 {
		received2[i] = map[string]bool{}
	}
	var connMu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			creds[idx] = mustVal(refreshAuth(creds[idx]))
			ticket := mustVal(getWSTicket(creds[idx].Access, group.ID))
			conn, err := dialWS(group.ID, ticket)
			if err != nil {
				fmt.Fprintf(os.Stderr, "reconnect ws dial %d failed: %v\n", idx, err)
				res.addError()
				return
			}
			connMu.Lock()
			conns2[idx] = conn
			connMu.Unlock()
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("reconnect ws connect phase failed", res)
	}
	return conns2, received2
}

func runCatchUp(concurrent int, creds []credentials, group groupRef,
	preCursor string, received2 []map[string]bool, res *result,
) {
	var totalCaughtUp int32
	var wg sync.WaitGroup
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			page := mustVal(getMessages(creds[idx].Access, group.ID, preCursor))
			for _, msg := range page.Items {
				if msg.Kind != "text" {
					continue
				}
				received2[idx][msg.Content] = true
			}
			atomic.AddInt32(&totalCaughtUp, int32(len(page.Items)))
			if len(page.Items) != concurrent {
				fmt.Fprintf(os.Stderr, "client %d catch-up got %d msgs, want %d\n",
					idx, len(page.Items), concurrent)
				res.addError()
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("  catch-up: %d total messages recovered across %d clients\n", totalCaughtUp, concurrent)
	if atomic.LoadInt32(&res.Errors) > 0 {
		fail("catch-up phase failed", res)
	}
}

func runLiveDelivery(concurrent int, conns2 []*websocket.Conn,
	anchor credentials, group groupRef, res *result,
) {
	type liveResult struct {
		seen map[string]bool
		errs int32
	}
	liveResults := make([]liveResult, concurrent)
	for i := range liveResults {
		liveResults[i].seen = map[string]bool{}
	}
	var liveWG sync.WaitGroup
	liveStart := make(chan struct{})

	for i := 0; i < concurrent; i++ {
		liveWG.Add(1)
		go func(idx int) {
			defer liveWG.Done()
			<-liveStart
			for j := 0; j < concurrent; j++ {
				_ = conns2[idx].SetReadDeadline(time.Now().Add(15 * time.Second))
				_, payload, err := conns2[idx].ReadMessage()
				if err != nil {
					liveResults[idx].errs++
					break
				}
				var msg wsMessage
				if err := json.Unmarshal(payload, &msg); err != nil || msg.Kind != "text" {
					j--
					continue
				}
				res.incRecv()
				if _, seen := liveResults[idx].seen[msg.Content]; seen {
					fmt.Fprintf(os.Stderr, "duplicate live msg '%s' on client %d\n", msg.Content, idx)
					res.addError()
				} else {
					liveResults[idx].seen[msg.Content] = true
				}
			}
		}(i)
	}

	anchorCreds := mustVal(refreshAuth(anchor))
	anchorTicket := mustVal(getWSTicket(anchorCreds.Access, group.ID))
	anchorConn, err := dialWS(group.ID, anchorTicket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anchor2 dial failed: %v\n", err)
		res.addError()
		close(liveStart)
		return
	}
	defer anchorConn.Close()

	liveMessages := make([]string, concurrent)
	tLiveStart := time.Now()
	close(liveStart)
	for i := 0; i < concurrent; i++ {
		content := fmt.Sprintf("live-%d-%s", i, randomSuffix())
		liveMessages[i] = content
		if err := anchorConn.WriteJSON(map[string]string{"content": content}); err != nil {
			fmt.Fprintf(os.Stderr, "anchor2 write %d failed: %v\n", i, err)
			res.addError()
			break
		}
		res.addSent()
	}
	liveWG.Wait()
	tLiveDuration := time.Since(tLiveStart)

	for i := 0; i < concurrent; i++ {
		if liveResults[i].errs > 0 || len(liveResults[i].seen) != concurrent {
			fmt.Fprintf(os.Stderr, "client %d live got %d/%d messages (errs=%d)\n",
				i, len(liveResults[i].seen), concurrent, liveResults[i].errs)
			res.addError()
		}
		for _, m := range liveMessages {
			if !liveResults[i].seen[m] {
				fmt.Fprintf(os.Stderr, "client %d missed live msg '%s'\n", i, m)
				res.addError()
			}
		}
	}
	fmt.Printf("  live delivery: %d msgs in %v\n", concurrent, tLiveDuration)
}

func printEvidence(res *result, r1Duration time.Duration) {
	fmt.Println()
	fmt.Println("====== REHEARSAL RESULTS ======")
	fmt.Printf("  total sent:       %d\n", atomic.LoadInt32(&res.Sent))
	fmt.Printf("  total received:   %d\n", atomic.LoadInt32(&res.Received))
	fmt.Printf("  errors:           %d\n", atomic.LoadInt32(&res.Errors))

	res.mu.Lock()
	lats := make([]int64, len(res.Latencies))
	copy(lats, res.Latencies)
	res.mu.Unlock()

	if len(lats) > 0 {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		p50 := lats[len(lats)/2]
		p95 := lats[int(math.Ceil(0.95*float64(len(lats))))-1]
		p99 := lats[int(math.Ceil(0.99*float64(len(lats))))-1]
		max := lats[len(lats)-1]
		min := lats[0]
		avg := int64(0)
		for _, l := range lats {
			avg += l
		}
		avg /= int64(len(lats))
		fmt.Printf("  r1 latency (μs):  min=%d avg=%d p50=%d p95=%d p99=%d max=%d\n",
			min, avg, p50, p95, p99, max)
	}
	totalRcvd := atomic.LoadInt32(&res.Received)
	if totalRcvd > 0 && r1Duration > 0 {
		throughput := float64(totalRcvd) / r1Duration.Seconds()
		fmt.Printf("  r1 throughput:    %.1f deliveries/sec\n", throughput)
	}

	fmt.Println("====== EXACT-ONCE VERIFICATION ======")
	if atomic.LoadInt32(&res.Errors) == 0 {
		fmt.Println("  PASS: no duplicates, no missing messages, no errors")
	} else {
		fmt.Printf("  FAIL: %d errors detected\n", atomic.LoadInt32(&res.Errors))
	}
	fmt.Println("====== REHEARSAL COMPLETE ======")
}
