package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func trafficDeliveryFixture() TrafficEvent {
	now := time.Now().UTC()
	return TrafficEvent{EventID: uuid.NewString(), UserID: 1, APIKeyID: 2, StartedAt: now, FinishedAt: now, UploadBytes: 3, DownloadBytes: 4, MeasurementVersion: 1, Endpoint: "/v1/messages", Method: "POST", StatusCode: 200, Outcome: "completed", RequestComplete: true}
}

func stoppedTrafficDelivery(t *testing.T) *TrafficDelivery {
	t.Helper()
	d, err := NewTrafficDelivery(t.TempDir(), func(context.Context, TrafficEvent) error { return nil })
	require.NoError(t, err)
	d.Stop()
	return d
}

func TestTrafficDeliveryQuarantinesPermanentMetadataAndContinues(t *testing.T) {
	for _, kind := range []string{"invalid_version", "invalid_uuid", "invalid_identity", "negative_bytes", "invalid_time", "null_metadata", "constraint", "data_exception"} {
		t.Run(kind, func(t *testing.T) {
			d := stoppedTrafficDelivery(t)
			bad, good := trafficDeliveryFixture(), trafficDeliveryFixture()
			switch kind {
			case "invalid_version":
				bad.MeasurementVersion = 2
			case "invalid_identity":
				bad.UserID = 0
			case "negative_bytes":
				bad.UploadBytes = -1
			case "invalid_time":
				bad.FinishedAt = bad.StartedAt.Add(-time.Second)
			case "null_metadata":
				bad.RequestID = "bad\x00id"
			}
			require.NoError(t, d.stage(bad))
			if kind == "invalid_uuid" {
				path := filepath.Join(d.dir, bad.EventID+".json")
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(d.dir, "bad.json"), []byte(strings.ReplaceAll(string(raw), bad.EventID, "bad")), 0600))
				require.NoError(t, os.Remove(path))
			}
			require.NoError(t, d.stage(good))
			delivered := []string{}
			d.write = func(_ context.Context, e TrafficEvent) error {
				if e.EventID == bad.EventID {
					if kind == "constraint" {
						return errors.Join(errors.New("write"), &pq.Error{Code: "23514"})
					}
					if kind == "data_exception" {
						return &pq.Error{Code: "22001"}
					}
					t.Fatal("invalid metadata reached database writer")
				}
				delivered = append(delivered, e.EventID)
				return nil
			}
			require.NoError(t, d.replay(context.Background()))
			require.Equal(t, []string{good.EventID}, delivered)
			files, err := filepath.Glob(filepath.Join(d.dir, "*.json.invalid"))
			require.NoError(t, err)
			require.Len(t, files, 1)
			health, err := d.Health(context.Background())
			require.NoError(t, err)
			require.Equal(t, "degraded", health.Status)
			require.Zero(t, health.PendingEvents)
			require.EqualValues(t, 1, health.QuarantinedEvents)
			require.NotNil(t, health.LastDeliveryAt)
		})
	}
}

func TestTrafficDeliveryRetainsRetryableErrors(t *testing.T) {
	for _, code := range []string{"08006", "40001", "42P01", "42501", ""} {
		t.Run(code, func(t *testing.T) {
			d := stoppedTrafficDelivery(t)
			event := trafficDeliveryFixture()
			require.NoError(t, d.stage(event))
			failure := error(&pq.Error{Code: pq.ErrorCode(code)})
			if code == "" {
				failure = context.DeadlineExceeded
			}
			d.write = func(context.Context, TrafficEvent) error { return failure }
			require.Error(t, d.replay(context.Background()))
			_, err := os.Stat(filepath.Join(d.dir, event.EventID+".json"))
			require.NoError(t, err)
			health, err := d.Health(context.Background())
			require.NoError(t, err)
			require.EqualValues(t, 1, health.PendingEvents)
			require.Zero(t, health.QuarantinedEvents)
			require.EqualValues(t, 1, health.DeliveryFailures)
			d.write = func(context.Context, TrafficEvent) error { return nil }
			require.NoError(t, d.replay(context.Background()))
			health, err = d.Health(context.Background())
			require.NoError(t, err)
			require.Zero(t, health.PendingEvents)
			require.Equal(t, "ok", health.Status)
		})
	}
}

func TestTrafficDeliveryHealthTracksBacklogAndFailures(t *testing.T) {
	d := stoppedTrafficDelivery(t)
	event := trafficDeliveryFixture()
	require.NoError(t, d.stage(event))
	old := time.Now().Add(-90 * time.Second)
	require.NoError(t, os.Chtimes(filepath.Join(d.dir, event.EventID+".json"), old, old))
	require.NoError(t, os.WriteFile(filepath.Join(d.dir, ".traffic-in-progress"), []byte("ignore"), 0600))
	health, err := d.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, "delayed", health.Status)
	require.EqualValues(t, 1, health.PendingEvents)
	require.GreaterOrEqual(t, health.OldestPendingAgeSeconds, int64(90))
	require.NotNil(t, health.OldestPendingAt)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = d.Health(ctx)
	require.ErrorIs(t, err, context.Canceled)
	d.dir = filepath.Join(d.dir, "missing")
	d.write = func(context.Context, TrafficEvent) error { return errors.New("offline") }
	d.RecordTraffic(trafficDeliveryFixture())
	require.EqualValues(t, 1, d.stagingFailures.Load())
	require.EqualValues(t, 1, d.persistenceFailures.Load())
}

func TestTrafficDeliveryConcurrentHealthAndRecording(t *testing.T) {
	d, err := NewTrafficDelivery(t.TempDir(), func(context.Context, TrafficEvent) error { return nil })
	require.NoError(t, err)
	defer d.Stop()
	var work sync.WaitGroup
	for i := 0; i < 4; i++ {
		work.Go(func() {
			for j := 0; j < 8; j++ {
				d.RecordTraffic(trafficDeliveryFixture())
				_, err := d.Health(context.Background())
				require.NoError(t, err)
			}
		})
	}
	work.Wait()
	require.Eventually(t, func() bool {
		h, err := d.Health(context.Background())
		return err == nil && h.PendingEvents == 0 && h.DeliveredEvents == 32
	}, 5*time.Second, 10*time.Millisecond)
}

func TestTrafficDeliveryFullBatchWakesRemainingBacklog(t *testing.T) {
	d := stoppedTrafficDelivery(t)
	for i := 0; i < 257; i++ {
		require.NoError(t, d.stage(trafficDeliveryFixture()))
	}
	require.NoError(t, d.replay(context.Background()))
	h, err := d.Health(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, h.PendingEvents)
	select {
	case <-d.wake:
	default:
		t.Fatal("full batch must wake the consumer even without new requests")
	}
	require.NoError(t, d.replay(context.Background()))
	h, err = d.Health(context.Background())
	require.NoError(t, err)
	require.Zero(t, h.PendingEvents)
}
