package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

/* TestChannelNotificationSinkReportsCompactionTrigger pins the baseline
 * compaction entry: the visible label names the compaction trigger (the fact
 * name), not the turn phase the reduction happened in. */
func TestChannelNotificationSinkReportsCompactionTrigger(t *testing.T) {
	events := make(chan tea.Msg, 1)
	sink := &channelNotificationSink{events: events, operationID: 1}
	sink.Notify(context.Background(), app.Notification{
		Kind: app.NotificationContextCompacted, Phase: "action", Name: "turn_context",
	})
	event, ok := (<-events).(runEventMsg)
	if !ok {
		t.Fatal("compaction notification produced no run event")
	}
	if !strings.Contains(event.body, "turn_context") {
		t.Fatalf("compaction event body = %q, want the compaction trigger", event.body)
	}
	if strings.Contains(event.body, "action") {
		t.Fatalf("compaction event body = %q, want no turn phase", event.body)
	}
}
