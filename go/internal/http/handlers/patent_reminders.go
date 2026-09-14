package handlers

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"time"
)

// Reminder facts are durable and idempotent per patent/deadline/stage. The UI
// also computes current urgency from server dates, so a late worker never hides
// a deadline and a paid period does not continue to appear as unpaid.
func StartPatentReminders(ctx context.Context, pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_, err := pool.Exec(runCtx, `WITH deadlines AS (
    SELECT p.id,x.kind,x.deadline,
      x.deadline-(now() AT TIME ZONE 'Asia/Shanghai')::date AS days
    FROM patent_records p CROSS JOIN LATERAL(VALUES('fee',p.fee_due),('expiry',p.expires_on)) x(kind,deadline)
    WHERE x.deadline IS NOT NULL AND x.deadline-(now() AT TIME ZONE 'Asia/Shanghai')::date<=p.reminder_days
   ), reminders AS (
    SELECT *,CASE WHEN days<0 THEN 'overdue' WHEN days=0 THEN 'today' WHEN days<=1 THEN 'tomorrow' WHEN days<=7 THEN 'week' WHEN days<=30 THEN 'month' ELSE 'advance' END AS stage FROM deadlines
   )
   INSERT INTO patent_events(patent_id,actor_id,action,detail)
   SELECT id,NULL,'reminder',jsonb_build_object('kind',kind,'deadline',deadline,'stage',stage,'days',days)
   FROM reminders ON CONFLICT DO NOTHING`)
			cancel()
			if err != nil {
				log.Printf("patent reminder scan: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}
