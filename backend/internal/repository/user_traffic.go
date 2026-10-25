package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

// One statement atomically inserts the event and increments its UTC hour only
// when newly inserted. Retrying after an unknown commit outcome is safe.
const insertTrafficSQL = `WITH inserted AS (
 INSERT INTO user_traffic_events
 (event_id,user_id,api_key_id,group_id,request_id,endpoint,method,started_at,finished_at,
 upload_bytes,download_bytes,status_code,outcome,request_complete,measurement_version)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
 ON CONFLICT (event_id) DO NOTHING
 RETURNING *
)
INSERT INTO user_traffic_hourly(bucket_start,user_id,api_key_id,group_id,upload_bytes,download_bytes,requests)
SELECT date_trunc('hour',started_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
 user_id,api_key_id,group_id,upload_bytes,download_bytes,1 FROM inserted
ON CONFLICT (user_id,api_key_id,group_id,bucket_start) DO UPDATE SET
 upload_bytes=user_traffic_hourly.upload_bytes+EXCLUDED.upload_bytes,
 download_bytes=user_traffic_hourly.download_bytes+EXCLUDED.download_bytes,
 requests=user_traffic_hourly.requests+EXCLUDED.requests`

func (r *usageLogRepository) RecordTraffic(ctx context.Context, e service.TrafficEvent) error {
	if r.sql == nil {
		return errors.New("traffic database unavailable")
	}
	if _, err := uuid.Parse(e.EventID); err != nil {
		return fmt.Errorf("invalid traffic event ID: %w", err)
	}
	if e.UserID <= 0 || e.APIKeyID <= 0 || e.UploadBytes < 0 || e.DownloadBytes < 0 ||
		e.MeasurementVersion != 1 || e.StartedAt.IsZero() || e.FinishedAt.Before(e.StartedAt) {
		return errors.New("invalid traffic event")
	}
	_, err := r.sql.ExecContext(ctx, insertTrafficSQL, e.EventID, e.UserID, e.APIKeyID, e.GroupID,
		e.RequestID, e.Endpoint, e.Method, e.StartedAt, e.FinishedAt, e.UploadBytes, e.DownloadBytes,
		e.StatusCode, e.Outcome, e.RequestComplete, e.MeasurementVersion)
	return err
}

func trafficScope(f service.TrafficFilter, args []any) (string, []any) {
	var conditions []string
	for _, v := range []struct {
		name string
		id   int64
	}{{"user_id", f.UserID}, {"api_key_id", f.APIKeyID}} {
		if v.id > 0 {
			args = append(args, v.id)
			conditions = append(conditions, fmt.Sprintf("%s=$%d", v.name, len(args)))
		}
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " AND " + strings.Join(conditions, " AND "), args
}

// Each local bucket independently combines whole UTC hours with raw boundary
// fragments. This preserves both DST hours and fractional-offset day boundaries.
func trafficSource(f service.TrafficFilter) (string, []any) {
	var values []string
	var args []any
	for _, b := range service.TrafficBuckets(f) {
		wholeStart := b.Start.UTC().Truncate(time.Hour)
		if wholeStart.Before(b.Start) {
			wholeStart = wholeStart.Add(time.Hour)
		}
		wholeEnd := b.End.UTC().Truncate(time.Hour)
		n := len(args)
		values = append(values, fmt.Sprintf("($%d::text,$%d::timestamptz,$%d::timestamptz,$%d::timestamptz,$%d::timestamptz)", n+1, n+2, n+3, n+4, n+5))
		args = append(args, b.Label, b.Start, b.End, wholeStart, wholeEnd)
	}
	scope, args := trafficScope(f, args)
	return `WITH buckets(label,start_at,end_at,whole_start,whole_end) AS (VALUES ` + strings.Join(values, ",") + `)
 SELECT b.label,COALESCE(SUM(t.upload_bytes),0),COALESCE(SUM(t.download_bytes),0),COALESCE(SUM(t.requests),0)
 FROM buckets b LEFT JOIN LATERAL (
 SELECT upload_bytes,download_bytes,requests FROM user_traffic_hourly
 WHERE bucket_start >= b.whole_start AND bucket_start < b.whole_end` + scope + `
 UNION ALL
 SELECT upload_bytes,download_bytes,1 AS requests FROM user_traffic_events
 WHERE started_at >= b.start_at AND started_at < b.end_at
 AND (started_at < b.whole_start OR started_at >= b.whole_end)` + scope + `
 ) t ON TRUE GROUP BY b.label,b.start_at ORDER BY b.start_at`, args
}

func (r *usageLogRepository) Traffic(ctx context.Context, f service.TrafficFilter) (*service.TrafficReport, error) {
	if f.Page < 1 || f.Page > 10000 || f.PageSize < 1 || f.PageSize > 100 || !f.End.After(f.Start) {
		return nil, errors.New("invalid traffic filter")
	}
	if r.db == nil {
		return nil, errors.New("traffic database unavailable")
	}
	// Totals, trend and page represent the same database snapshot even while new
	// events arrive. Reads never hold a lock over request processing.
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	out := &service.TrafficReport{Items: []service.TrafficEvent{}, Trend: []service.TrafficPoint{},
		Page: f.Page, PageSize: f.PageSize, Period: f.Period, Start: f.Start, End: f.End,
		Timezone: timezone.Location().String(), MeasurementVersion: 1}
	if f.Location != nil {
		out.Timezone = f.Location.String()
	}
	source, args := trafficSource(f)
	rows, err := tx.QueryContext(ctx, source, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p service.TrafficPoint
		if err := rows.Scan(&p.Bucket, &p.UploadBytes, &p.DownloadBytes, &p.Requests); err != nil {
			_ = rows.Close()
			return nil, err
		}
		p.TotalBytes = p.UploadBytes + p.DownloadBytes
		out.UploadBytes += p.UploadBytes
		out.DownloadBytes += p.DownloadBytes
		out.Requests += p.Requests
		out.Trend = append(out.Trend, p)
	}
	out.TotalBytes = out.UploadBytes + out.DownloadBytes
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	scope, detailArgs := trafficScope(f, []any{f.Start, f.End})
	detailArgs = append(detailArgs, f.PageSize+1, (f.Page-1)*f.PageSize)
	rows, err = tx.QueryContext(ctx, `SELECT event_id,user_id,api_key_id,group_id,request_id,endpoint,method,
 started_at,finished_at,upload_bytes,download_bytes,status_code,outcome,request_complete,measurement_version
 FROM user_traffic_events WHERE started_at >= $1 AND started_at < $2`+scope+
		fmt.Sprintf(` ORDER BY started_at DESC,event_id DESC LIMIT $%d OFFSET $%d`, len(detailArgs)-1, len(detailArgs)), detailArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e service.TrafficEvent
		if err := rows.Scan(&e.EventID, &e.UserID, &e.APIKeyID, &e.GroupID, &e.RequestID, &e.Endpoint, &e.Method,
			&e.StartedAt, &e.FinishedAt, &e.UploadBytes, &e.DownloadBytes, &e.StatusCode, &e.Outcome, &e.RequestComplete, &e.MeasurementVersion); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out.Items = append(out.Items, e)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(out.Items) > f.PageSize {
		out.HasMore = true
		out.Items = out.Items[:f.PageSize]
	}
	scope, sinceArgs := trafficScope(f, nil)
	var since sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT MIN(started_at) FROM user_traffic_events WHERE TRUE`+scope, sinceArgs...).Scan(&since); err != nil {
		return nil, err
	}
	if since.Valid {
		out.RecordedSince = &since.Time
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
