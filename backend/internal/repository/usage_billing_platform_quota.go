package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func incrementUsageBillingPlatformQuota(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) error {
	if !domain.IsConcretePlatform(cmd.PlatformQuotaPlatform) || cmd.PlatformUsageAt.IsZero() {
		return service.ErrUsageBillingRequestConflict
	}
	var id int64
	var monthly sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT id,monthly_window_start FROM user_platform_quotas
		WHERE user_id=$1 AND platform=$2 AND deleted_at IS NULL
		AND (daily_limit_usd IS NOT NULL OR weekly_limit_usd IS NOT NULL OR monthly_limit_usd IS NOT NULL) FOR UPDATE`,
		cmd.UserID, cmd.PlatformQuotaPlatform).Scan(&id, &monthly)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // absent quota is unlimited; never create limits implicitly
	}
	if err != nil {
		return err
	}
	at := cmd.PlatformUsageAt
	month := at
	if monthly.Valid && at.Before(monthly.Time.Add(30*24*time.Hour)) {
		month = monthly.Time
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_platform_quota_totals
		(quota_id,daily_usage_usd,weekly_usage_usd,monthly_usage_usd,daily_window_start,weekly_window_start,monthly_window_start,legacy_daily_anchor,legacy_weekly_anchor,legacy_monthly_anchor)
		VALUES($1,$2,$2,CASE WHEN $5>$6 THEN 0 ELSE $2 END,$3,$4,$5,$3,$4,$5)
		ON CONFLICT(quota_id) DO UPDATE SET
		daily_usage_usd=CASE WHEN usage_platform_quota_totals.daily_window_start>EXCLUDED.daily_window_start THEN usage_platform_quota_totals.daily_usage_usd
		 WHEN usage_platform_quota_totals.daily_window_start<EXCLUDED.daily_window_start THEN EXCLUDED.daily_usage_usd
		 ELSE usage_platform_quota_totals.daily_usage_usd+EXCLUDED.daily_usage_usd END,
		weekly_usage_usd=CASE WHEN usage_platform_quota_totals.weekly_window_start>EXCLUDED.weekly_window_start THEN usage_platform_quota_totals.weekly_usage_usd
		 WHEN usage_platform_quota_totals.weekly_window_start<EXCLUDED.weekly_window_start THEN EXCLUDED.weekly_usage_usd
		 ELSE usage_platform_quota_totals.weekly_usage_usd+EXCLUDED.weekly_usage_usd END,
		monthly_usage_usd=CASE WHEN usage_platform_quota_totals.monthly_window_start>$6 THEN usage_platform_quota_totals.monthly_usage_usd
		 WHEN usage_platform_quota_totals.monthly_window_start+INTERVAL '30 days'<=$6 THEN EXCLUDED.monthly_usage_usd
		 ELSE usage_platform_quota_totals.monthly_usage_usd+EXCLUDED.monthly_usage_usd END,
		daily_window_start=GREATEST(usage_platform_quota_totals.daily_window_start,EXCLUDED.daily_window_start),
		weekly_window_start=GREATEST(usage_platform_quota_totals.weekly_window_start,EXCLUDED.weekly_window_start),
		monthly_window_start=CASE WHEN usage_platform_quota_totals.monthly_window_start+INTERVAL '30 days'<=$6 THEN $6 ELSE usage_platform_quota_totals.monthly_window_start END`,
		id, cmd.PlatformQuotaCost, timezone.StartOfDay(at), timezone.StartOfWeek(at), month, at)
	return err
}
