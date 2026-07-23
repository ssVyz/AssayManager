package web

import (
	"context"
	"time"

	"AssayManager/internal/analysis"
	"AssayManager/internal/store"
)

const schedulerTick = time.Minute

// StartScheduler runs the recurring-job scheduler in the background until ctx is
// cancelled. It fires due jobs once at startup and then on each tick.
func (s *Server) StartScheduler(ctx context.Context) {
	go func() {
		s.scheduleTick(ctx)
		t := time.NewTicker(schedulerTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.scheduleTick(ctx)
			}
		}
	}()
}

func (s *Server) scheduleTick(ctx context.Context) {
	now := time.Now().UTC()
	due, err := s.store.DueSchedules(now)
	if err != nil {
		s.log.Error("scheduler: list due", "err", err)
		return
	}
	for _, sch := range due {
		if ctx.Err() != nil {
			return
		}
		s.fireSchedule(sch, now)
	}
}

// fireSchedule advances a job's next execution (always relative to now, so
// missed cycles are skipped rather than replayed) and, if the job can run,
// enqueues it as a background run. If it can't run — assay gone/ineligible,
// BLAST unavailable — it is silently skipped; next_execution has already moved.
func (s *Server) fireSchedule(sch store.Schedule, now time.Time) {
	// Advance first: never double-fire, and a skip still moves forward.
	if err := s.store.AdvanceSchedule(sch.ID, now.AddDate(0, 0, sch.IntervalDays)); err != nil {
		s.log.Error("scheduler: advance", "schedule", sch.ID, "err", err)
		return
	}

	if sch.Method != "blast" || !s.analyzer.BlastAvailable() {
		s.log.Warn("scheduler: skipping (method unsupported or BLAST unavailable)", "schedule", sch.ID, "method", sch.Method)
		return
	}

	// Run the latest version of the anchor assay's lineage.
	anchor, err := s.store.AssayByID(sch.OwnerID, sch.AssayID)
	if err != nil {
		s.log.Warn("scheduler: anchor assay missing; skipping", "schedule", sch.ID, "err", err)
		return
	}
	assay, err := s.store.LatestAssayByName(sch.OwnerID, anchor.Name)
	if err != nil {
		s.log.Warn("scheduler: no assay version; skipping", "schedule", sch.ID, "err", err)
		return
	}
	query, taxids, verr := blastInputsFromAssay(assay.Content)
	if verr != nil || validateAssayForAnalysis(assay.Content) != nil {
		s.log.Warn("scheduler: assay not BLAST-eligible; skipping", "schedule", sch.ID, "assay", assay.Name)
		return
	}
	owner, err := s.store.UserByID(sch.OwnerID)
	if err != nil {
		s.log.Warn("scheduler: owner missing; skipping", "schedule", sch.ID, "err", err)
		return
	}

	from, to := lookbackRange(now, sch.LookbackMonths)
	req := analysis.Request{
		AssayJSON: []byte(assay.Content),
		Blast: &analysis.BlastParams{
			Query:       query,
			TaxIDs:      taxids,
			From:        from,
			To:          to,
			MinCoverage: owner.BlastMinCoverage,
			MinIdentity: owner.BlastMinIdentity,
			HitlistSize: owner.BlastHitlistSize,
		},
	}
	resultID, err := s.store.CreateRun(sch.OwnerID, assay, store.NewRun{
		ReferenceName: blastDescriptor(taxids, from, to),
		Source:        "blast",
		BlastFrom:     from,
		BlastTo:       to,
		ScheduleID:    sch.ID,
	})
	if err != nil {
		s.log.Error("scheduler: create run", "schedule", sch.ID, "err", err)
		return
	}
	s.log.Info("scheduler: started run", "schedule", sch.ID, "result", resultID, "assay", assay.Name, "version", assay.Version)
	go s.runAnalysis(resultID, req, "")
}
