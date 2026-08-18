package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/require"
)

var _ JobQueue = (*stubQueue)(nil)

type stubQueue struct {
	mu         sync.Mutex
	enqueued   []NotificationJob
	enqueueErr error
	complete   []string
	retried    []NotificationJob
	retryAt    []time.Time
	failed     []string
	claimJobs  []NotificationJob
	claimIdx   int
	purgeCalls int
	statsCalls int
	purge      JobPurgeResult
	stats      JobQueueStats
	purgeErr   error
	statsErr   error
}

func (q *stubQueue) Enqueue(_ context.Context, userID string, evt NotificationEvent) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.enqueueErr != nil {
		return q.enqueueErr
	}
	q.enqueued = append(q.enqueued, NotificationJob{
		JobID:       "job-1",
		UserID:      userID,
		Event:       evt,
		Attempts:    0,
		MaxAttempts: maxPushAttempts,
	})
	return nil
}

func (q *stubQueue) Claim(context.Context) (*NotificationJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.claimIdx >= len(q.claimJobs) {
		return nil, nil
	}
	job := q.claimJobs[q.claimIdx]
	q.claimIdx++
	return &job, nil
}
func (q *stubQueue) Complete(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.complete = append(q.complete, jobID)
	return nil
}
func (q *stubQueue) Retry(_ context.Context, jobID string, attempts int, nextAttemptAt time.Time, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.retried = append(q.retried, NotificationJob{JobID: jobID, Attempts: attempts})
	q.retryAt = append(q.retryAt, nextAttemptAt)
	return nil
}
func (q *stubQueue) Fail(_ context.Context, jobID string, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failed = append(q.failed, jobID)
	return nil
}
func (q *stubQueue) ReclaimStale(context.Context, time.Duration) error { return nil }

func (q *stubQueue) PurgeExpired(context.Context) (JobPurgeResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.purgeCalls++
	return q.purge, q.purgeErr
}

func (q *stubQueue) Stats(context.Context) (JobQueueStats, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.statsCalls++
	return q.stats, q.statsErr
}

func TestSendToUser_EnqueuesWithoutCallingProviders(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		t.Fatal("deliver must not run on the request path")
		return nil
	}

	start := time.Now()
	err := s.SendToUser(context.Background(), "11111111-1111-1111-1111-111111111111", NotificationEvent{
		Type:  "review",
		Title: "Review submitted",
	})
	r.NoError(err)
	r.Less(time.Since(start), 100*time.Millisecond)
	r.Len(q.enqueued, 1)
	r.Equal("review", q.enqueued[0].Event.Type)
}

func TestSendToUser_DoesNotWaitWhenEnqueueFails(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{enqueueErr: errors.New("db down")}
	s := newSender(nil, q, false)

	err := s.SendToUser(context.Background(), "11111111-1111-1111-1111-111111111111", NotificationEvent{Type: "checkin"})
	r.EqualError(err, "db down")
}

func TestProcessJob_CompletesOnSuccess(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error { return nil }

	s.processJob(&NotificationJob{
		JobID:       "job-ok",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "submission"},
		MaxAttempts: 5,
	})
	r.Equal([]string{"job-ok"}, q.complete)
	r.Empty(q.retried)
	r.Empty(q.failed)
}

func TestProcessJob_RetriesWhenPreferencesFail(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(&pushRepoStub{prefsErr: errors.New("db down")}, q, false)

	s.processJob(&NotificationJob{
		JobID:       "job-prefs",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		MaxAttempts: 5,
	})
	r.Empty(q.complete)
	r.Len(q.retried, 1)
}

func TestProcessJob_RetriesThenFails(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		return errors.New("expo timeout")
	}

	s.processJob(&NotificationJob{
		JobID:       "job-retry",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		Attempts:    0,
		MaxAttempts: 3,
	})
	r.Empty(q.complete)
	r.Len(q.retried, 1)
	r.Equal(1, q.retried[0].Attempts)

	s.processJob(&NotificationJob{
		JobID:       "job-retry",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		Attempts:    2,
		MaxAttempts: 3,
	})
	r.Equal([]string{"job-retry"}, q.failed)
}

func TestRetryDelay_GrowsAndCaps(t *testing.T) {
	r := require.New(t)
	r.Equal(2*time.Second, retryDelay(1))
	r.Equal(4*time.Second, retryDelay(2))
	r.Equal(5*time.Minute, retryDelay(12))
}

func TestMaintain_PurgesAndLogsQueueStats(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{
		purge: JobPurgeResult{DeletedDone: 3, DeletedFailed: 1},
		stats: JobQueueStats{Pending: 2, Failed: 4, Retried: 1, RetryAttempts: 5},
	}
	s := newSender(nil, q, false)

	s.maintain(context.Background())
	r.Equal(1, q.purgeCalls)
	r.Equal(1, q.statsCalls)
}

func TestMaintain_StillReadsStatsWhenPurgeFails(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{
		purgeErr: errors.New("purge down"),
		stats:    JobQueueStats{Pending: 8},
	}
	s := newSender(nil, q, false)

	s.maintain(context.Background())
	r.Equal(1, q.purgeCalls)
	r.Equal(1, q.statsCalls)
}

func TestDrain_ProcessesJobsConcurrently(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{
		claimJobs: []NotificationJob{
			{JobID: "j1", UserID: "u1", MaxAttempts: 5},
			{JobID: "j2", UserID: "u2", MaxAttempts: 5},
			{JobID: "j3", UserID: "u3", MaxAttempts: 5},
			{JobID: "j4", UserID: "u4", MaxAttempts: 5},
		},
	}
	s := newSender(nil, q, false)
	var current, max atomic.Int32
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		n := current.Add(1)
		for {
			old := max.Load()
			if n <= old || max.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(60 * time.Millisecond)
		current.Add(-1)
		return nil
	}

	s.drain(context.Background())
	s.inFlight.Wait()

	r.GreaterOrEqual(max.Load(), int32(2))
	r.Len(q.complete, 4)
}

func TestProcessJob_CircuitOpenUsesCooldownRetry(t *testing.T) {
	r := require.New(t)
	q := &stubQueue{}
	s := newSender(nil, q, false)
	s.deliverFn = func(context.Context, string, NotificationEvent) error {
		return errCircuitOpen
	}

	before := time.Now().UTC()
	s.processJob(&NotificationJob{
		JobID:       "job-open",
		UserID:      "u1",
		Event:       NotificationEvent{Type: "review"},
		MaxAttempts: 5,
	})
	r.Len(q.retried, 1)
	r.GreaterOrEqual(q.retryAt[0].Sub(before), breakerOpenFor-time.Second)
}

type pushRepoStub struct {
	mu               sync.Mutex
	prefs            *NotificationPreferences
	prefsErr         error
	web              []WebPushSubscriptionRequest
	ios              []IOSPushTokenRequest
	deletedEndpoints []string
	deletedTokens    []string
}

func (r *pushRepoStub) UpsertPreferences(context.Context, string, NotificationPreferencesRequest) error {
	return nil
}
func (r *pushRepoStub) GetPreferences(context.Context, string) (*NotificationPreferences, error) {
	if r.prefsErr != nil {
		return nil, r.prefsErr
	}
	if r.prefs != nil {
		return r.prefs, nil
	}
	return &NotificationPreferences{NotificationsEnabled: true}, nil
}
func (r *pushRepoStub) UpsertWebPushSubscription(context.Context, string, WebPushSubscriptionRequest) error {
	return nil
}
func (r *pushRepoStub) DeleteWebPushSubscription(_ context.Context, _ string, endpoint string) error {
	return r.DeleteWebPushSubscriptions(context.Background(), "", []string{endpoint})
}
func (r *pushRepoStub) DeleteWebPushSubscriptions(_ context.Context, _ string, endpoints []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedEndpoints = append(r.deletedEndpoints, endpoints...)
	return nil
}
func (r *pushRepoStub) UpsertIOSPushToken(context.Context, string, IOSPushTokenRequest) error {
	return nil
}
func (r *pushRepoStub) DeleteIOSPushToken(_ context.Context, _ string, token string) error {
	return r.DeleteIOSPushTokens(context.Background(), "", []string{token})
}
func (r *pushRepoStub) DeleteIOSPushTokens(_ context.Context, _ string, tokens []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletedTokens = append(r.deletedTokens, tokens...)
	return nil
}
func (r *pushRepoStub) ListWebPushSubscriptions(context.Context, string) ([]WebPushSubscriptionRequest, error) {
	return r.web, nil
}
func (r *pushRepoStub) ListIOSPushTokens(context.Context, string) ([]IOSPushTokenRequest, error) {
	return r.ios, nil
}

func TestSendExpoPush_BatchesTokensAndDropsStale(t *testing.T) {
	r := require.New(t)
	t.Setenv("EXPO_ACCESS_TOKEN", "test-token")

	var requests atomic.Int32
	var seen [][]string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		var msgs []expoPushMessage
		if err := json.NewDecoder(req.Body).Decode(&msgs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		tos := make([]string, len(msgs))
		tickets := make([]map[string]any, len(msgs))
		for i, msg := range msgs {
			tos[i] = msg.To
			if strings.Contains(msg.To, "stale") {
				tickets[i] = map[string]any{
					"status":  "error",
					"message": "not registered",
					"details": map[string]any{"error": "DeviceNotRegistered"},
				}
				continue
			}
			tickets[i] = map[string]any{"status": "ok", "id": fmt.Sprintf("id-%d", i)}
		}
		mu.Lock()
		seen = append(seen, tos)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": tickets})
	}))
	defer srv.Close()

	repo := &pushRepoStub{
		ios: []IOSPushTokenRequest{
			{Token: "ExponentPushToken[ok-1]"},
			{Token: "ExponentPushToken[stale]"},
			{Token: "ExponentPushToken[ok-2]"},
		},
	}
	s := newSender(repo, &stubQueue{}, false)
	s.expoPushURL = srv.URL

	err := s.sendExpoPush(context.Background(), "11111111-1111-1111-1111-111111111111", NotificationEvent{
		Type:  "review",
		Title: "Hello",
		Body:  "World",
	})
	r.NoError(err)
	r.Equal(int32(1), requests.Load())
	r.Equal([]string{"ExponentPushToken[stale]"}, repo.deletedTokens)
	mu.Lock()
	r.Len(seen, 1)
	r.Equal([]string{
		"ExponentPushToken[ok-1]",
		"ExponentPushToken[stale]",
		"ExponentPushToken[ok-2]",
	}, seen[0])
	mu.Unlock()
}

func TestSendExpoPush_ChunksOverMaxBatch(t *testing.T) {
	r := require.New(t)
	t.Setenv("EXPO_ACCESS_TOKEN", "test-token")

	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var msgs []expoPushMessage
		if err := json.NewDecoder(req.Body).Decode(&msgs); err != nil || len(msgs) > expoPushMaxBatch {
			http.Error(w, "bad batch", http.StatusBadRequest)
			return
		}
		tickets := make([]map[string]any, len(msgs))
		for i := range msgs {
			tickets[i] = map[string]any{"status": "ok", "id": fmt.Sprintf("%d", i)}
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": tickets})
	}))
	defer srv.Close()

	tokens := make([]IOSPushTokenRequest, expoPushMaxBatch+1)
	for i := range tokens {
		tokens[i] = IOSPushTokenRequest{Token: fmt.Sprintf("ExponentPushToken[%d]", i)}
	}
	s := newSender(&pushRepoStub{ios: tokens}, &stubQueue{}, false)
	s.expoPushURL = srv.URL

	r.NoError(s.sendExpoPush(context.Background(), "u1", NotificationEvent{Title: "Hi"}))
	r.Equal(int32(2), requests.Load())
}

func TestSendExpoPush_TicketErrorFailsWithoutDeleting(t *testing.T) {
	r := require.New(t)
	t.Setenv("EXPO_ACCESS_TOKEN", "test-token")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"status": "ok", "id": "1"},
				{"status": "error", "message": "too big", "details": map[string]any{"error": "MessageTooBig"}},
			},
		})
	}))
	defer srv.Close()

	repo := &pushRepoStub{
		ios: []IOSPushTokenRequest{
			{Token: "ExponentPushToken[ok]"},
			{Token: "ExponentPushToken[big]"},
		},
	}
	s := newSender(repo, &stubQueue{}, false)
	s.expoPushURL = srv.URL

	err := s.sendExpoPush(context.Background(), "u1", NotificationEvent{Title: "Hi"})
	r.Error(err)
	r.Contains(err.Error(), "too big")
	r.Empty(repo.deletedTokens)
}

func TestSendWebPush_DropsGoneSubscriptions(t *testing.T) {
	r := require.New(t)
	t.Setenv("WEB_PUSH_VAPID_PRIVATE_KEY", "test-private")
	t.Setenv("WEB_PUSH_VAPID_PUBLIC_KEY", "test-public")

	repo := &pushRepoStub{
		web: []WebPushSubscriptionRequest{
			{Endpoint: "https://push.example/ok"},
			{Endpoint: "https://push.example/gone"},
		},
	}
	s := newSender(repo, &stubQueue{}, false)
	s.webPushSend = func(_ context.Context, _ []byte, sub WebPushSubscriptionRequest, _ *webpush.Options) error {
		if strings.Contains(sub.Endpoint, "gone") {
			return errStalePushDestination
		}
		return nil
	}

	err := s.sendWebPush(context.Background(), "u1", NotificationEvent{Type: "checkin", Title: "Hi"})
	r.NoError(err)
	r.Equal([]string{"https://push.example/gone"}, repo.deletedEndpoints)
}

func TestChunkByAndUniqueStrings(t *testing.T) {
	r := require.New(t)
	r.Nil(chunkBy([]int{}, 2))
	r.Equal([][]int{{1, 2}, {3}}, chunkBy([]int{1, 2, 3}, 2))
	r.Equal([]string{"a", "b"}, uniqueStrings([]string{" a ", "a", "", "b"}))
}

func TestClassifyExpoTicket(t *testing.T) {
	r := require.New(t)
	r.NoError(classifyExpoTicket(expoPushTicket{Status: "ok"}))
	r.Error(classifyExpoTicket(expoPushTicket{}))
	r.Error(classifyExpoTicket(expoPushTicket{Status: "error"}))

	stale := expoPushTicket{Status: "error", Message: "gone"}
	stale.Details.Error = "DeviceNotRegistered"
	r.ErrorIs(classifyExpoTicket(stale), errStalePushDestination)

	invalid := expoPushTicket{Status: "error"}
	invalid.Details.Error = "InvalidPushToken"
	r.ErrorIs(classifyExpoTicket(invalid), errStalePushDestination)

	rate := expoPushTicket{Status: "error", Message: "slow down"}
	rate.Details.Error = "MessageRateExceeded"
	err := classifyExpoTicket(rate)
	r.ErrorIs(err, errProviderOutage)
}

func TestParseExpoTickets(t *testing.T) {
	r := require.New(t)
	tickets, err := parseExpoTickets([]byte(`{"data":[{"status":"ok","id":"1"},{"status":"ok","id":"2"}]}`), 2)
	r.NoError(err)
	r.Len(tickets, 2)

	_, err = parseExpoTickets([]byte(`{"errors":[{"code":"API_ERROR","message":"nope"}]}`), 1)
	r.EqualError(err, "expo api error: nope")

	_, err = parseExpoTickets([]byte(`{"data":{"status":"ok","id":"1"}}`), 1)
	r.NoError(err)
}
