package main

import "time"

func waitForReadiness(access, groupID string, ready []string) bool {
	want := make(map[string]bool, len(ready))
	for _, content := range ready {
		want[content] = true
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		page, err := getMessages(access, groupID, "")
		if err == nil {
			for _, msg := range page.Items {
				delete(want, msg.Content)
			}
			if len(want) == 0 {
				return true
			}
		}
		// Polling is state-based: the next request observes persistence rather
		// than masking a race with an unconditional sleep.
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
