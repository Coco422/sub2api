package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

// TrafficDelivery stages completed metadata on persistent local storage before
// asynchronous SQL delivery. A bounded wake channel is only a hint; files remain
// authoritative across overflow, database failure and process restarts.
type TrafficDelivery struct {
	dir                 string
	write               func(context.Context, TrafficEvent) error
	wake                chan struct{}
	cancel              context.CancelFunc
	done                chan struct{}
	once                sync.Once
	delivered           atomic.Int64
	deliveryFailures    atomic.Int64
	stagingFailures     atomic.Int64
	persistenceFailures atomic.Int64
	lastDelivery        atomic.Int64
}

func NewTrafficDelivery(dir string, write func(context.Context, TrafficEvent) error) (*TrafficDelivery, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	d := &TrafficDelivery{dir: dir, write: write, wake: make(chan struct{}, 1), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go d.run(ctx)
	return d, nil
}

func (d *TrafficDelivery) Stop() { d.once.Do(func() { d.cancel(); <-d.done }) }

func (d *TrafficDelivery) RecordTraffic(e TrafficEvent) {
	if err := d.stage(e); err != nil {
		d.stagingFailures.Add(1)
		// A storage outage must be visible. A direct database write is the
		// fallback, not a silent in-memory enqueue that can disappear.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if dbErr := d.write(ctx, e); dbErr != nil {
			d.deliveryFailures.Add(1)
			d.persistenceFailures.Add(1)
			logger.LegacyPrintf("user_traffic", "METERING LOSS event=%s staging=%v database=%v", e.EventID, err, dbErr)
		} else {
			d.noteDelivery()
			logger.LegacyPrintf("user_traffic", "local staging failed; database persisted event=%s: %v", e.EventID, err)
		}
		return
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *TrafficDelivery) stage(e TrafficEvent) error {
	if _, err := uuid.Parse(e.EventID); err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(d.dir, ".traffic-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = f.Close(); _ = os.Remove(name) }()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(d.dir, e.EventID+".json")); err != nil {
		return err
	}
	dir, err := os.Open(d.dir)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func (d *TrafficDelivery) replay(ctx context.Context) error {
	dir, err := os.Open(d.dir)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	// Scan in bounded batches without loading an outage's entire backlog.
	for processed := 0; processed < 256 && ctx.Err() == nil; {
		entries, readErr := dir.ReadDir(64)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			processed++
			path := filepath.Join(d.dir, entry.Name())
			f, err := os.Open(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			raw, err := io.ReadAll(io.LimitReader(f, 8193))
			_ = f.Close()
			if err != nil {
				return err
			}
			var e TrafficEvent
			if len(raw) > 8192 || json.Unmarshal(raw, &e) != nil || e.EventID+".json" != entry.Name() || ValidateTrafficEvent(e) != nil {
				if err := d.quarantine(path); err != nil {
					return err
				}
				continue
			}
			if err = d.write(ctx, e); err != nil {
				d.deliveryFailures.Add(1)
				if permanentTrafficError(err) {
					if err := d.quarantine(path); err != nil {
						return err
					}
					continue
				}
				return err
			}
			d.noteDelivery()
			if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if processed >= 256 {
			select {
			case d.wake <- struct{}{}:
			default:
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return ctx.Err()
}

func (d *TrafficDelivery) run(ctx context.Context) {
	defer close(d.done)
	ticker := time.NewTicker(2 * time.Second)
	healthTicker := time.NewTicker(30 * time.Second)
	defer healthTicker.Stop()
	defer ticker.Stop()
	for {
		cycle, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := d.replay(cycle)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("user_traffic", "delivery pending; will retry: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-healthTicker.C:
			healthCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			health, healthErr := d.Health(healthCtx)
			cancel()
			if healthErr != nil {
				logger.LegacyPrintf("user_traffic", "delivery health scan failed: %v", healthErr)
			} else if health.Status != "ok" {
				logger.LegacyPrintf("user_traffic", "DELIVERY ALERT status=%s pending=%d oldest_age_seconds=%d quarantined=%d persistence_failures=%d", health.Status, health.PendingEvents, health.OldestPendingAgeSeconds, health.QuarantinedEvents, health.PersistenceFailures)
			}
		case <-ticker.C:
		case <-d.wake:
		}
	}
}

func (d *TrafficDelivery) quarantine(path string) error {
	if err := os.Rename(path, path+".invalid"); err != nil {
		return fmt.Errorf("quarantine traffic event: %w", err)
	}
	logger.LegacyPrintf("user_traffic", "DELIVERY ALERT invalid pending event quarantined; manual repair required: %s", filepath.Base(path))
	return nil
}

func permanentTrafficError(err error) bool {
	if errors.Is(err, ErrInvalidTrafficEvent) {
		return true
	}
	var state interface{ SQLState() string }
	// Data exceptions and integrity violations cannot be fixed by retrying the
	// same metadata. Network, transaction, schema and permission errors retry.
	if errors.As(err, &state) {
		return strings.HasPrefix(state.SQLState(), "22") || strings.HasPrefix(state.SQLState(), "23")
	}
	return false
}

func (d *TrafficDelivery) noteDelivery() {
	d.delivered.Add(1)
	d.lastDelivery.Store(time.Now().Unix())
}

// TrafficDeliveryHealth is node-local; counters reset when this process restarts.
type TrafficDeliveryHealth struct {
	Status                  string     `json:"status"`
	PendingEvents           int64      `json:"pending_events"`
	QuarantinedEvents       int64      `json:"quarantined_events"`
	OldestPendingAt         *time.Time `json:"oldest_pending_at"`
	OldestPendingAgeSeconds int64      `json:"oldest_pending_age_seconds"`
	DeliveredEvents         int64      `json:"delivered_events"`
	DeliveryFailures        int64      `json:"delivery_failures"`
	StagingFailures         int64      `json:"staging_failures"`
	PersistenceFailures     int64      `json:"persistence_failures"`
	LastDeliveryAt          *time.Time `json:"last_delivery_at"`
	CheckedAt               time.Time  `json:"checked_at"`
}

func (d *TrafficDelivery) Health(ctx context.Context) (TrafficDeliveryHealth, error) {
	h := TrafficDeliveryHealth{Status: "ok", CheckedAt: time.Now().UTC(), DeliveredEvents: d.delivered.Load(), DeliveryFailures: d.deliveryFailures.Load(), StagingFailures: d.stagingFailures.Load(), PersistenceFailures: d.persistenceFailures.Load()}
	if last := d.lastDelivery.Load(); last > 0 {
		t := time.Unix(last, 0).UTC()
		h.LastDeliveryAt = &t
	}
	dir, err := os.Open(d.dir)
	if err != nil {
		return h, err
	}
	defer func() { _ = dir.Close() }()
	for {
		if err := ctx.Err(); err != nil {
			return h, err
		}
		entries, err := dir.ReadDir(64)
		if err != nil && !errors.Is(err, io.EOF) {
			return h, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".json.invalid") {
				h.QuarantinedEvents++
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			info, err := entry.Info()
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return h, err
			}
			h.PendingEvents++
			modified := info.ModTime().UTC()
			if h.OldestPendingAt == nil || modified.Before(*h.OldestPendingAt) {
				h.OldestPendingAt = &modified
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if h.OldestPendingAt != nil {
		h.OldestPendingAgeSeconds = int64(h.CheckedAt.Sub(*h.OldestPendingAt).Seconds())
		if h.OldestPendingAgeSeconds < 0 {
			h.OldestPendingAgeSeconds = 0
		}
	}
	if h.PendingEvents >= 256 || h.OldestPendingAgeSeconds >= 60 {
		h.Status = "delayed"
	}
	if h.QuarantinedEvents > 0 || h.PersistenceFailures > 0 {
		h.Status = "degraded"
	}
	return h, nil
}
