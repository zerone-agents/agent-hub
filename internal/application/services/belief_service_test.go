package services

import (
	"strings"
	"testing"
	"time"

	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
)

func newBeliefTestService(t *testing.T) (*BeliefService, *RunService, string) {
	t.Helper()
	runService, _ := newRunTestService(t)
	require.NoError(t, NewBeliefService(runService).EnsureSchemas())
	require.NoError(t, NewBeliefService(runService).EnsureSchemas(), "EnsureSchemas must be idempotent")
	r, err := runService.Create("t1", CreateRunInput{Name: "belief-run"})
	require.NoError(t, err)
	return NewBeliefService(runService), runService, r.ID
}

func TestBeliefServiceRecordDeliveryCreatesBeliefAndIdempotentReplay(t *testing.T) {
	svc, _, runID := newBeliefTestService(t)
	at := time.Now().UTC()

	require.NoError(t, svc.RecordDelivery("t1", runID, 7, "fact:scandal", at, "del-1"))
	beliefs, err := svc.List("t1", runID, 7, "")
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, "fact:scandal", beliefs[0].FactRef)
	require.Equal(t, "known", beliefs[0].Status)
	require.Equal(t, 50, beliefs[0].Confidence)
	require.Equal(t, "delivery", beliefs[0].Source)

	// Same idempotency key replay: no-op, not an error, no state change.
	require.NoError(t, svc.RecordDelivery("t1", runID, 7, "fact:scandal", at, "del-1"))
	beliefs, err = svc.List("t1", runID, 7, "")
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, 50, beliefs[0].Confidence)
	require.Equal(t, uint64(1), beliefs[0].Revision)

	// A genuinely repeated delivery (new key) is confirming evidence.
	require.NoError(t, svc.RecordDelivery("t1", runID, 7, "fact:scandal", at.Add(time.Hour), "del-2"))
	beliefs, err = svc.List("t1", runID, 7, "")
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, "believed", beliefs[0].Status)
	require.Equal(t, 60, beliefs[0].Confidence)
}

func TestBeliefServiceListFiltersFactRefAndExcludesForgotten(t *testing.T) {
	svc, _, runID := newBeliefTestService(t)
	at := time.Now().UTC()
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:a", at, "a1"))
	// fact:b was delivered long ago; lazy decay on read settles it below the
	// forgotten threshold (50 - 7×5 = 15 < 20).
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:b", at.Add(-7*24*time.Hour), "b1"))

	beliefs, err := svc.List("t1", runID, 1, "")
	require.NoError(t, err)
	require.Len(t, beliefs, 1, "forgotten beliefs are excluded by default")
	require.Equal(t, "fact:a", beliefs[0].FactRef)

	filtered, err := svc.List("t1", runID, 1, "fact:b")
	require.NoError(t, err)
	require.Empty(t, filtered)

	all, err := svc.listBeliefs("t1", runID, 1, "", true)
	require.NoError(t, err)
	require.Len(t, all, 2)
	for _, b := range all {
		if b.FactRef == "fact:b" {
			require.Equal(t, "forgotten", b.Status)
		}
	}

	// Only the holder sees the belief; another agent sees nothing.
	other, err := svc.List("t1", runID, 2, "")
	require.NoError(t, err)
	require.Empty(t, other)
}

func TestBeliefServiceAddEvidenceRequiresDeliveryFirst(t *testing.T) {
	svc, _, runID := newBeliefTestService(t)
	at := time.Now().UTC()
	err := svc.AddEvidence("t1", runID, 1, "fact:never-delivered", false, at, "e1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "尚未送达")
}

func TestBeliefServiceEvidenceIsIdempotentAndDisputeLadder(t *testing.T) {
	svc, _, runID := newBeliefTestService(t)
	at := time.Now().UTC()
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:x", at, "x1"))

	require.NoError(t, svc.AddEvidence("t1", runID, 1, "fact:x", true, at.Add(time.Hour), "x-c1"))
	require.NoError(t, svc.AddEvidence("t1", runID, 1, "fact:x", true, at.Add(time.Hour), "x-c1"), "replayed key must be a no-op")
	beliefs, err := svc.List("t1", runID, 1, "fact:x")
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, "doubted", beliefs[0].Status)
	require.Equal(t, 25, beliefs[0].Confidence)

	// Confirming evidence on a doubted belief raises confidence but keeps
	// the status (only known/forgotten promote to believed).
	require.NoError(t, svc.AddEvidence("t1", runID, 1, "fact:x", false, at.Add(2*time.Hour), "x-e1"))
	beliefs, err = svc.List("t1", runID, 1, "fact:x")
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, "doubted", beliefs[0].Status)
	require.Equal(t, 35, beliefs[0].Confidence)
}

func TestBeliefServiceClaimCreatesClaimScopedBelief(t *testing.T) {
	svc, _, runID := newBeliefTestService(t)
	at := time.Now().UTC()

	ref, err := svc.Claim("t1", runID, 3, "fact:scandal", "我声称这件事另有隐情", at, "claim-1")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ref, "claim:"))

	// Replay with the same key returns the same factRef without side effects.
	ref2, err := svc.Claim("t1", runID, 3, "fact:scandal", "我声称这件事另有隐情", at, "claim-1")
	require.NoError(t, err)
	require.Equal(t, ref, ref2)

	beliefs, err := svc.List("t1", runID, 3, ref)
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, "believed", beliefs[0].Status)
	require.Equal(t, "claim", beliefs[0].Source)
	require.Equal(t, "我声称这件事另有隐情", beliefs[0].Statement)

	_, err = svc.Claim("t1", runID, 3, "fact:scandal", "  ", at, "claim-2")
	require.Error(t, err)
}

func TestBeliefServiceDisputesDetectsDivergentStances(t *testing.T) {
	// Fresh arc: contradiction walks the ladder to disputed and stays
	// absorbing even at confidence 0 — visible because no decay days have
	// elapsed between the event and the read.
	svc, _, runID := newBeliefTestService(t)
	now := time.Now().UTC()
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:scandal", now, "f-1"))
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:scandal", now.Add(time.Hour), "f-1b"))
	require.NoError(t, svc.RecordDelivery("t1", runID, 2, "fact:scandal", now, "f-2"))
	require.NoError(t, svc.AddEvidence("t1", runID, 2, "fact:scandal", true, now.Add(2*time.Hour), "f-2c1"))
	require.NoError(t, svc.AddEvidence("t1", runID, 2, "fact:scandal", true, now.Add(3*time.Hour), "f-2c2"))

	disputes, err := svc.Disputes("t1", runID)
	require.NoError(t, err)
	require.Len(t, disputes, 1)
	require.Equal(t, "fact:scandal", disputes[0].FactRef)
	require.Len(t, disputes[0].Entries, 2)
	statuses := map[uint64]string{}
	for _, e := range disputes[0].Entries {
		statuses[e.AgentID] = e.Status
	}
	require.Equal(t, "believed", statuses[1])
	require.Equal(t, "disputed", statuses[2])
	require.Equal(t, 0, confidenceOf(t, disputes[0].Entries, 2))
}

func TestBeliefServiceDisputeDissolvesWhenHolderForgets(t *testing.T) {
	// Decay arc: agent 2's events sit ~45h in the past. Lazy decay on read
	// settles their confidence below the forgotten threshold, and a
	// forgotten holder no longer counts toward the dispute.
	svc, _, runID := newBeliefTestService(t)
	now := time.Now().UTC()
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:scandal", now, "s-1"))
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:scandal", now.Add(time.Hour), "s-1b"))

	aged := now.Add(-47 * time.Hour)
	require.NoError(t, svc.RecordDelivery("t1", runID, 2, "fact:scandal", aged, "s-2"))
	require.NoError(t, svc.AddEvidence("t1", runID, 2, "fact:scandal", true, aged.Add(2*time.Hour), "s-2c"))

	disputes, err := svc.Disputes("t1", runID)
	require.NoError(t, err)
	require.Len(t, disputes, 1, "believed vs doubted is a dispute")
	statuses := map[uint64]string{}
	for _, e := range disputes[0].Entries {
		statuses[e.AgentID] = e.Status
	}
	require.Equal(t, "believed", statuses[1])
	require.Equal(t, "doubted", statuses[2])

	// Another contradiction drops confidence to 0 (absorbing disputed in
	// storage); at ~44h old, lazy decay on this read settles it below the
	// forgotten threshold and agent 2 drops out of the dispute.
	require.NoError(t, svc.AddEvidence("t1", runID, 2, "fact:scandal", true, aged.Add(3*time.Hour), "s-2c2"))
	disputes, err = svc.Disputes("t1", runID)
	require.NoError(t, err)
	require.Empty(t, disputes, "decayed-to-forgotten holders no longer dispute")
}

func confidenceOf(t *testing.T, entries []DisputeEntry, agentID uint64) int {
	t.Helper()
	for _, e := range entries {
		if e.AgentID == agentID {
			return e.Confidence
		}
	}
	t.Fatalf("agent %d not in dispute entries", agentID)
	return 0
}

func TestBeliefServiceRunAndTenantIsolation(t *testing.T) {
	svc, runService, runID := newBeliefTestService(t)
	r2, err := runService.Create("t1", CreateRunInput{Name: "belief-run-2"})
	require.NoError(t, err)
	at := time.Now().UTC()
	require.NoError(t, svc.RecordDelivery("t1", runID, 1, "fact:scandal", at, "iso-1"))

	// Cross-run isolation: the belief is invisible in run 2.
	beliefs, err := svc.List("t1", r2.ID, 1, "")
	require.NoError(t, err)
	require.Empty(t, beliefs)
	disputes, err := svc.Disputes("t1", r2.ID)
	require.NoError(t, err)
	require.Empty(t, disputes)

	// Cross-tenant isolation: tenant t2 cannot see t1's state.
	beliefs, err = svc.List("t2", runID, 1, "")
	require.NoError(t, err)
	require.Empty(t, beliefs)
}

func TestBeliefServiceFreezeRespectsRunLifecycle(t *testing.T) {
	svc, runService, runID := newBeliefTestService(t)
	at := time.Now().UTC()
	_, err := runService.Transition("t1", runID, rundomain.StatusRunning)
	require.NoError(t, err)
	_, err = runService.Transition("t1", runID, rundomain.StatusCompleted)
	require.NoError(t, err)
	err = svc.RecordDelivery("t1", runID, 1, "fact:frozen", at, "frozen-1")
	require.ErrorIs(t, err, rundomain.ErrFrozen)
}
