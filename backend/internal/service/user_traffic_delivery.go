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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

// TrafficDelivery stages completed metadata on persistent local storage before
// asynchronous SQL delivery. A bounded wake channel is only a hint; files remain
// authoritative across overflow, database failure and process restarts.
type TrafficDelivery struct {
	dir    string
	write  func(context.Context, TrafficEvent) error
	wake   chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
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
		// A storage outage must be visible. A direct database write is the
		// fallback, not a silent in-memory enqueue that can disappear.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if dbErr := d.write(ctx, e); dbErr != nil {
			logger.LegacyPrintf("user_traffic", "METERING LOSS event=%s staging=%v database=%v", e.EventID, err, dbErr)
		} else {
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
			var e TrafficEvent
			if err != nil || len(raw) > 8192 || json.Unmarshal(raw, &e) != nil || e.EventID+".json" != entry.Name() {
				if err := os.Rename(path, path+".invalid"); err != nil {
					return fmt.Errorf("quarantine traffic event: %w", err)
				}
				logger.LegacyPrintf("user_traffic", "invalid pending event quarantined; manual repair required: %s", entry.Name())
				continue
			}
			if err = d.write(ctx, e); err != nil {
				return err
			}
			if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
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
		case <-ticker.C:
		case <-d.wake:
		}
	}
}
