package control

import (
	"testing"
	"time"
)

func TestAlertInboxDedupAndRouting(t *testing.T) {
	inbox := NewAlertInbox()
	first := inbox.Ingest(AlertIngest{
		EventType: "external.alert.disk",
		Message:   "disk full",
		Severity:  "critical",
		Fields:    map[string]any{"host": "db-01", "service": "storage"},
	})
	if !first.Created || first.Item.Route != "pager" {
		t.Fatalf("unexpected first alert ingest: %+v", first)
	}

	second := inbox.Ingest(AlertIngest{
		EventType: "external.alert.disk",
		Message:   "disk full",
		Severity:  "high",
		Fields:    map[string]any{"host": "db-01", "service": "storage"},
	})
	if !second.Deduplicated || second.Item.Count != 2 {
		t.Fatalf("expected deduplication with count increment: %+v", second)
	}
	if second.Item.Severity != "critical" {
		t.Fatalf("expected severity to remain at highest level, got %s", second.Item.Severity)
	}
}

func TestAlertInboxSuppressionWindow(t *testing.T) {
	inbox := NewAlertInbox()
	fp := "external.alert|service=api|host=node-1|msg=latency high"
	if _, err := inbox.Suppress(fp, 2*time.Minute, "maintenance window"); err != nil {
		t.Fatalf("suppress failed: %v", err)
	}

	res := inbox.Ingest(AlertIngest{
		Fingerprint: fp,
		EventType:   "external.alert",
		Message:     "latency high",
		Severity:    "high",
		Fields:      map[string]any{"service": "api", "host": "node-1"},
	})
	if !res.Suppressed {
		t.Fatalf("expected ingested alert to be suppressed: %+v", res)
	}
	if got := inbox.Summary().Total; got != 0 {
		t.Fatalf("expected suppression to avoid creating inbox item, got total=%d", got)
	}

	if ok := inbox.ClearSuppression(fp); !ok {
		t.Fatalf("expected suppression clear to succeed")
	}
	res = inbox.Ingest(AlertIngest{
		Fingerprint: fp,
		EventType:   "external.alert",
		Message:     "latency high",
		Severity:    "high",
		Fields:      map[string]any{"service": "api", "host": "node-1"},
	})
	if !res.Created {
		t.Fatalf("expected alert creation after suppression clear: %+v", res)
	}
}

func TestAlertInboxIngestEventInference(t *testing.T) {
	inbox := NewAlertInbox()

	_, ok := inbox.IngestEvent(Event{Type: "queue.saturation.predicted", Message: "queue pressure rising"})
	if !ok {
		t.Fatalf("expected saturation event to be ingested")
	}
	_, ok = inbox.IngestEvent(Event{Type: "normal.lifecycle", Message: "heartbeat"})
	if ok {
		t.Fatalf("did not expect non-alert event ingestion")
	}

	items := inbox.List("all", 10)
	if len(items) != 1 {
		t.Fatalf("expected one inferred alert item, got %d", len(items))
	}
	if items[0].Route != "chatops" {
		t.Fatalf("expected medium severity route to chatops, got %s", items[0].Route)
	}
}

func TestAlertInboxRoutingPolicyOverride(t *testing.T) {
	inbox := NewAlertInbox()

	updated, err := inbox.SetRoutingPolicy(AlertRoutingPolicy{
		CriticalRoute: "pager",
		HighRoute:     "pager",
		MediumRoute:   "ticket",
		LowRoute:      "digest",
	})
	if err != nil {
		t.Fatalf("set routing policy failed: %v", err)
	}
	if updated.HighRoute != "pager" || updated.MediumRoute != "ticket" {
		t.Fatalf("unexpected routing policy: %+v", updated)
	}
	if _, err := inbox.SetRoutingPolicy(AlertRoutingPolicy{
		CriticalRoute: "unknown",
		HighRoute:     "ticket",
		MediumRoute:   "chatops",
		LowRoute:      "digest",
	}); err == nil {
		t.Fatalf("expected invalid routing policy route failure")
	}

	res := inbox.Ingest(AlertIngest{
		EventType: "external.alert.latency",
		Message:   "latency high",
		Severity:  "high",
		Fields:    map[string]any{"service": "api"},
	})
	if res.Item.Route != "pager" {
		t.Fatalf("expected high severity route override to pager, got %+v", res.Item)
	}
}
