package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCronJobFromRaw_PopulatesEveryColumn(t *testing.T) {
	suspend := false
	last := time.Now().Add(-2 * time.Minute)
	item := rawCronJobItem{}
	item.Metadata.Name = "nightly"
	item.Metadata.Namespace = "ops"
	item.Metadata.CreationTimestamp = time.Now().Add(-3 * time.Hour)
	item.Spec.Schedule = "0 2 * * *"
	item.Spec.Suspend = &suspend
	item.Status.LastScheduleTime = &last
	item.Status.Active = []struct {
		Name string `json:"name"`
	}{{Name: "run-1"}, {Name: "run-2"}}

	got := cronJobFromRaw(item)
	if got.Name != "nightly" || got.Namespace != "ops" {
		t.Errorf("identity wrong: %+v", got)
	}
	if got.Schedule != "0 2 * * *" {
		t.Errorf("schedule = %q", got.Schedule)
	}
	if got.Suspend {
		t.Error("expected suspend=false")
	}
	if got.Active != 2 {
		t.Errorf("active = %d, want 2", got.Active)
	}
	if got.LastSchedule == cronJobNeverScheduled {
		t.Errorf("last schedule should be a duration, got %q", got.LastSchedule)
	}
}

func TestCronJobFromRaw_SuspendNilTreatedAsFalse(t *testing.T) {
	item := rawCronJobItem{}
	item.Metadata.Name = "x"
	if got := cronJobFromRaw(item); got.Suspend {
		t.Error("nil suspend pointer must render as false (kubectl default)")
	}
}

func TestCronJobFromRaw_SuspendTrueSurfaces(t *testing.T) {
	suspend := true
	item := rawCronJobItem{}
	item.Spec.Suspend = &suspend
	if got := cronJobFromRaw(item); !got.Suspend {
		t.Error("explicit suspend=true must propagate")
	}
}

func TestCronJobLastScheduleAge_NeverScheduledShowsSentinel(t *testing.T) {
	if got := cronJobLastScheduleAge(nil); got != cronJobNeverScheduled {
		t.Errorf("nil time = %q, want %q", got, cronJobNeverScheduled)
	}
	zero := time.Time{}
	if got := cronJobLastScheduleAge(&zero); got != cronJobNeverScheduled {
		t.Errorf("zero time = %q, want %q", got, cronJobNeverScheduled)
	}
}

func TestCronJobLastScheduleAge_RecentScheduleProducesAge(t *testing.T) {
	t10 := time.Now().Add(-10 * time.Minute)
	got := cronJobLastScheduleAge(&t10)
	if got == cronJobNeverScheduled {
		t.Error("recent schedule must not return the never-scheduled sentinel")
	}
	if got == "" {
		t.Error("recent schedule must produce a non-empty age")
	}
}

func TestDecodeCronJobObject_RoundTripsAndRejectsMalformed(t *testing.T) {
	const payload = `{
		"metadata": {"name": "n", "namespace": "ns"},
		"spec": {"schedule": "@hourly", "suspend": true},
		"status": {"active": []}
	}`
	got, err := decodeCronJobObject(json.RawMessage(payload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Schedule != "@hourly" {
		t.Errorf("schedule = %q", got.Schedule)
	}
	if !got.Suspend {
		t.Error("suspend=true did not survive decode")
	}
	if _, err := decodeCronJobObject(json.RawMessage("{bad")); err == nil {
		t.Error("expected error on malformed JSON")
	}
}

func TestFetchCronJobs_HappyPath(t *testing.T) {
	const stdout = `{
		"items": [
			{
				"metadata": {"name": "n", "namespace": "ns"},
				"spec": {"schedule": "* * * * *"},
				"status": {"active": []}
			}
		]
	}`
	withFakePathKubectl(t, `printf '%s' '`+stdout+`'`)
	msg, ok := FetchCronJobs("ns")().(CronJobsMsg)
	if !ok {
		t.Fatalf("expected CronJobsMsg, got %T", FetchCronJobs("ns")())
	}
	if msg.Err != nil {
		t.Fatalf("unexpected err: %v", msg.Err)
	}
	if len(msg.CronJobs) != 1 || msg.CronJobs[0].Schedule != "* * * * *" {
		t.Errorf("payload wrong: %+v", msg.CronJobs)
	}
}

func TestFetchCronJobs_PropagatesKubectlError(t *testing.T) {
	withFakePathKubectl(t, `printf 'cronjob boom' 1>&2; exit 1`)
	msg, ok := FetchCronJobs("ns")().(CronJobsMsg)
	if !ok {
		t.Fatalf("expected CronJobsMsg, got %T", FetchCronJobs("ns")())
	}
	if msg.Err == nil {
		t.Fatal("expected kubectl error")
	}
	if !strings.Contains(msg.Err.Error(), "boom") {
		t.Errorf("error should preserve stderr; got %v", msg.Err)
	}
	if len(msg.CronJobs) != 0 {
		t.Errorf("error path should not return rows; got %d", len(msg.CronJobs))
	}
}
