package repository

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Opt-in, isolated pg_temp schema. No production migrations or data are touched.
func TestTrafficPostgresIdempotencyIsolationAndCalendarBuckets(t *testing.T) {
	dsn := os.Getenv("TRAFFIC_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TRAFFIC_TEST_POSTGRES_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.ExecContext(ctx, "SET search_path TO pg_temp")
	require.NoError(t, err)
	migration, err := os.ReadFile("../../migrations/307_user_traffic.sql")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = db.ExecContext(ctx, strings.ReplaceAll(string(migration), "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE IF NOT EXISTS"))
		require.NoError(t, err)
	}
	repo := newUsageLogRepositoryWithSQL(nil, db)
	_, err = db.ExecContext(ctx, `CREATE TEMP TABLE api_keys(id BIGINT PRIMARY KEY,user_id BIGINT,name TEXT,deleted_at TIMESTAMPTZ,key TEXT);
 INSERT INTO api_keys VALUES (2,1,'Current key',NULL,'never-return-this-secret'),(3,1,'Old key',NOW(),'deleted-secret'),(10,9,'Other key',NULL,'other-secret')`)
	require.NoError(t, err)
	choices, err := repo.TrafficKeyOptions(ctx, 1, "")
	require.NoError(t, err)
	require.Equal(t, []service.TrafficKeyOption{{ID: 2, Name: "Current key"}, {ID: 3, Name: "Old key", Deleted: true}}, choices)
	choices, err = repo.TrafficKeyOptions(ctx, 1, "old")
	require.NoError(t, err)
	require.Equal(t, []service.TrafficKeyOption{{ID: 3, Name: "Old key", Deleted: true}}, choices)
	choices, err = repo.TrafficKeyOptions(ctx, 1, "#3")
	require.NoError(t, err)
	require.Equal(t, []service.TrafficKeyOption{{ID: 3, Name: "Old key", Deleted: true}}, choices)
	choices, err = repo.TrafficKeyOptions(ctx, 1, "#10")
	require.NoError(t, err)
	require.Empty(t, choices)
	choices, err = repo.TrafficKeyOptions(ctx, 1, "Other")
	require.NoError(t, err)
	require.Empty(t, choices)
	choices, err = repo.TrafficKeyOptions(ctx, 0, "Other")
	require.NoError(t, err)
	require.Equal(t, []service.TrafficKeyOption{{ID: 10, Name: "Other key"}}, choices)
	loc, err := time.LoadLocation("Asia/Kathmandu")
	require.NoError(t, err)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	e := service.TrafficEvent{EventID: uuid.NewString(), UserID: 1, APIKeyID: 2, GroupID: 3, RequestID: "same", Endpoint: "/v1/messages", Method: "POST", StartedAt: start.Add(5 * time.Minute), FinishedAt: start.Add(6 * time.Minute), UploadBytes: 100, DownloadBytes: 200, StatusCode: 200, Outcome: "completed", RequestComplete: true, MeasurementVersion: 1}
	require.NoError(t, repo.RecordTraffic(ctx, e))
	require.NoError(t, repo.RecordTraffic(ctx, e))
	// Opposite sides of a non-whole-hour midnight must land on different days.
	e2 := e
	e2.EventID = uuid.NewString()
	e2.StartedAt = start.AddDate(0, 0, 1).Add(-5 * time.Minute)
	e2.FinishedAt = e2.StartedAt.Add(time.Minute)
	require.NoError(t, repo.RecordTraffic(ctx, e2))
	e3 := e
	e3.EventID = uuid.NewString()
	e3.StartedAt = start.AddDate(0, 0, 1).Add(5 * time.Minute)
	e3.FinishedAt = e3.StartedAt.Add(time.Minute)
	require.NoError(t, repo.RecordTraffic(ctx, e3))
	other := e
	other.EventID = uuid.NewString()
	other.UserID = 9
	other.APIKeyID = 10
	require.NoError(t, repo.RecordTraffic(ctx, other))
	f := service.TrafficFilter{UserID: 1, Start: start, End: start.AddDate(0, 1, 0), Period: "month", Granularity: "day", Page: 1, PageSize: 1, Location: loc}
	report, err := repo.Traffic(ctx, f)
	require.NoError(t, err)
	require.EqualValues(t, 3, report.Requests)
	require.EqualValues(t, 300, report.UploadBytes)
	require.EqualValues(t, 600, report.DownloadBytes)
	require.EqualValues(t, 900, report.TotalBytes)
	require.Len(t, report.Trend, 31)
	require.EqualValues(t, 2, report.Trend[0].Requests)
	require.EqualValues(t, 1, report.Trend[1].Requests)
	require.Len(t, report.Items, 1)
	require.True(t, report.HasMore)
	require.NotNil(t, report.RecordedSince)
	f.Page = 2
	page, err := repo.Traffic(ctx, f)
	require.NoError(t, err)
	require.NotEqual(t, report.Items[0].EventID, page.Items[0].EventID)
	f.APIKeyID = 10
	report, err = repo.Traffic(ctx, f)
	require.NoError(t, err)
	require.Zero(t, report.Requests)
	require.Empty(t, report.Items)
	require.Nil(t, report.RecordedSince)
	// Event and rollup updates must roll back together if the latter fails.
	_, err = db.ExecContext(ctx, `ALTER TABLE user_traffic_hourly ADD CONSTRAINT reject_fixture CHECK (user_id <> 99)`)
	require.NoError(t, err)
	broken := e
	broken.EventID = uuid.NewString()
	broken.UserID = 99
	require.Error(t, repo.RecordTraffic(ctx, broken))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_traffic_events WHERE event_id=$1`, broken.EventID).Scan(&count))
	require.Zero(t, count)
	// DST's repeated hour remains two distinct buckets and is counted once each.
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	f = service.TrafficFilter{UserID: 1, Start: time.Date(2026, 11, 1, 0, 0, 0, 0, ny), End: time.Date(2026, 11, 2, 0, 0, 0, 0, ny), Period: "day", Granularity: "hour", Page: 1, PageSize: 20, Location: ny}
	for _, offset := range []time.Duration{90 * time.Minute, 150 * time.Minute} {
		item := e
		item.EventID = uuid.NewString()
		item.StartedAt = f.Start.Add(offset)
		item.FinishedAt = item.StartedAt.Add(time.Minute)
		require.NoError(t, repo.RecordTraffic(ctx, item))
	}
	report, err = repo.Traffic(ctx, f)
	require.NoError(t, err)
	require.Len(t, report.Trend, 25)
	require.EqualValues(t, 2, report.Requests)
	require.NotEqual(t, report.Trend[1].Bucket, report.Trend[2].Bucket)

	// A skipped civil day must neither hang bucket construction nor leave a
	// hole in the byte totals on the two adjacent existing dates.
	apia, err := time.LoadLocation("Pacific/Apia")
	require.NoError(t, err)
	f = service.TrafficFilter{UserID: 11, Start: time.Date(2011, 12, 1, 0, 0, 0, 0, apia), End: time.Date(2012, 1, 1, 0, 0, 0, 0, apia), Period: "month", Granularity: "day", Page: 1, PageSize: 20, Location: apia}
	for _, day := range []int{29, 31} {
		item := e
		item.EventID = uuid.NewString()
		item.UserID = 11
		item.StartedAt = time.Date(2011, 12, day, 12, 0, 0, 0, apia)
		item.FinishedAt = item.StartedAt.Add(time.Minute)
		require.NoError(t, repo.RecordTraffic(ctx, item))
	}
	report, err = repo.Traffic(ctx, f)
	require.NoError(t, err)
	require.Len(t, report.Trend, 30)
	require.EqualValues(t, 2, report.Requests)
	require.EqualValues(t, 600, report.TotalBytes)
	require.Equal(t, "2011-12-29", report.Trend[28].Bucket)
	require.Equal(t, "2011-12-31", report.Trend[29].Bucket)
	require.EqualValues(t, 1, report.Trend[28].Requests)
	require.EqualValues(t, 1, report.Trend[29].Requests)
}

// A falling/idle/rebounding series must remain independent in every view.
func TestTrafficPostgresTrendBucketsAreIndependent(t *testing.T) {
	dsn := os.Getenv("TRAFFIC_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TRAFFIC_TEST_POSTGRES_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.ExecContext(ctx, "SET search_path TO pg_temp")
	require.NoError(t, err)
	migration, err := os.ReadFile("../../migrations/307_user_traffic.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, strings.ReplaceAll(string(migration), "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE IF NOT EXISTS"))
	require.NoError(t, err)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	loc, err := time.LoadLocation("Asia/Kathmandu")
	require.NoError(t, err)
	for i, period := range []string{"day", "week", "month", "year"} {
		t.Run(period, func(t *testing.T) {
			start := time.Date(2026, 10, 26, 0, 0, 0, 0, loc)
			end, granularity := start.AddDate(0, 0, 1), "hour"
			switch period {
			case "week":
				end, granularity = start.AddDate(0, 0, 7), "day"
			case "month":
				start = time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
				end, granularity = start.AddDate(0, 1, 0), "day"
			case "year":
				start = time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
				end, granularity = start.AddDate(1, 0, 0), "month"
			}
			filter := service.TrafficFilter{UserID: int64(20 + i), Start: start, End: end, Period: period, Granularity: granularity, Page: 1, PageSize: 20, Location: loc}
			buckets := service.TrafficBuckets(filter)
			amounts := []int64{900, 100, 0, 400}
			for index, amount := range amounts {
				if amount == 0 {
					continue
				}
				// Exercise raw boundary fragments and full UTC-hour rollups together.
				for _, offset := range []time.Duration{2 * time.Minute, 50 * time.Minute} {
					at := buckets[index].Start.Add(offset)
					event := service.TrafficEvent{EventID: uuid.NewString(), UserID: filter.UserID, APIKeyID: filter.UserID, Endpoint: "/v1/messages", Method: "POST", StartedAt: at, FinishedAt: at, UploadBytes: amount / 2, DownloadBytes: amount, StatusCode: 200, Outcome: "completed", RequestComplete: true, MeasurementVersion: 1}
					require.NoError(t, repo.RecordTraffic(ctx, event))
				}
			}
			report, err := repo.Traffic(ctx, filter)
			require.NoError(t, err)
			require.Len(t, report.Trend, len(buckets))
			var uploadSum, downloadSum int64
			for index, point := range report.Trend {
				expected := int64(0)
				if index < len(amounts) {
					expected = amounts[index]
				}
				require.Equal(t, buckets[index].Label, point.Bucket)
				require.Equal(t, expected, point.UploadBytes, "bucket %s", point.Bucket)
				require.Equal(t, expected*2, point.DownloadBytes, "bucket %s", point.Bucket)
				require.Equal(t, expected*3, point.TotalBytes, "bucket %s", point.Bucket)
				uploadSum += point.UploadBytes
				downloadSum += point.DownloadBytes
			}
			require.EqualValues(t, 1400, report.UploadBytes)
			require.EqualValues(t, 2800, report.DownloadBytes)
			require.EqualValues(t, 4200, report.TotalBytes)
			require.EqualValues(t, 6, report.Requests)
			require.Equal(t, uploadSum, report.UploadBytes)
			require.Equal(t, downloadSum, report.DownloadBytes)
		})
	}
}
