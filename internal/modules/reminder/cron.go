package reminder

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/robfig/cron/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/telegram/botapi"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

// HourlySchedule matches the legacy Cloud Scheduler expression: every hour on
// the hour, evaluated in Asia/Jakarta.
const HourlySchedule = "0 * * * *"

// LockStaleAfter is how long a held lock may go un-refreshed before another
// instance may reclaim it. A crash mid-job therefore delays the next run by at
// most this long, which is acceptable for reminders.
const LockStaleAfter = 10 * time.Minute

// lockDocPath identifies the leader lock document.
const (
	lockCollection = "cron_locks"
	lockDocument   = "hourly_reminder"
)

// ErrLockHeld indicates another instance holds a fresh lock.
var ErrLockHeld = errors.New("cron lock held by another instance")

// CronManager runs the scheduled reminder jobs.
//
// During the migration both the legacy Cloud Functions backend and this Go
// backend may be deployed at once. Without coordination both would fire the
// hourly job and users would receive duplicate Telegram reminders, so every run
// is gated on a Firestore leader lock.
type CronManager struct {
	db             *firestore.Client
	scheduler      *cron.Cron
	instanceID     string
	bot            *botapi.Client
	accountService *account.Service
}

// NewCronManager constructs a manager whose schedule is evaluated in
// Asia/Jakarta. instanceID identifies this process in the leader lock and should
// be stable for the lifetime of the process.
func NewCronManager(
	db *firestore.Client,
	instanceID string,
	bot *botapi.Client,
	accountService *account.Service,
) *CronManager {
	return &CronManager{
		db:             db,
		scheduler:      cron.New(cron.WithLocation(datetime.Location())),
		instanceID:     instanceID,
		bot:            bot,
		accountService: accountService,
	}
}

// Start registers the hourly job and begins scheduling. It returns an error if
// the schedule expression is rejected, rather than silently running no jobs.
func (cm *CronManager) Start() error {
	if _, err := cm.scheduler.AddFunc(HourlySchedule, cm.runHourly); err != nil {
		return err
	}
	cm.scheduler.Start()
	slog.Info("cron scheduler started",
		"schedule", HourlySchedule,
		"timezone", datetime.Location().String(),
		"instanceId", cm.instanceID)
	return nil
}

// Stop halts scheduling and waits for a running job to finish.
func (cm *CronManager) Stop() {
	ctx := cm.scheduler.Stop()
	<-ctx.Done()
	slog.Info("cron scheduler stopped")
}

func (cm *CronManager) runHourly() {
	ctx := context.Background()

	if err := cm.acquireLeaderLock(ctx); err != nil {
		if errors.Is(err, ErrLockHeld) {
			slog.Info("skipping hourly reminders, lock held elsewhere")
			return
		}
		slog.Error("skipping hourly reminders, lock acquisition failed", "error", err)
		return
	}
	defer cm.releaseLeaderLock(ctx)

	slog.Info("hourly reminder run started", "instanceId", cm.instanceID)

	cm.processRoutineExpenseReminders(ctx)
	cm.processDailyNoTransactionReminders(ctx)

	slog.Info("hourly reminder run finished", "instanceId", cm.instanceID)
}

// acquireLeaderLock claims the lock, reclaiming it if the current holder's
// timestamp is older than LockStaleAfter. Returns ErrLockHeld when another
// instance holds a fresh lock.
func (cm *CronManager) acquireLeaderLock(ctx context.Context) error {
	ref := cm.db.Collection(lockCollection).Doc(lockDocument)

	return cm.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}

		now := time.Now()

		if snap != nil && snap.Exists() {
			var data struct {
				LockedAt time.Time `firestore:"lockedAt"`
				LockedBy string    `firestore:"lockedBy"`
			}
			if err := snap.DataTo(&data); err == nil {
				if data.LockedBy != cm.instanceID && now.Sub(data.LockedAt) < LockStaleAfter {
					return ErrLockHeld
				}
			}
		}

		return tx.Set(ref, map[string]interface{}{
			"lockedBy": cm.instanceID,
			"lockedAt": now,
		})
	})
}

// releaseLeaderLock deletes the lock only if this instance still holds it, so a
// slow run cannot delete a lock another instance has since reclaimed.
func (cm *CronManager) releaseLeaderLock(ctx context.Context) {
	ref := cm.db.Collection(lockCollection).Doc(lockDocument)

	err := cm.db.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return err
		}

		var data struct {
			LockedBy string `firestore:"lockedBy"`
		}
		if err := snap.DataTo(&data); err != nil {
			return nil
		}
		if data.LockedBy != cm.instanceID {
			return nil
		}
		return tx.Delete(ref)
	})
	if err != nil {
		slog.Error("failed to release cron lock", "error", err)
	}
}
