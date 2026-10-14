package sqs

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func TestHumanSeconds(t *testing.T) {
	cases := map[string]string{"0": "0s", "30": "30s", "300": "5m", "7200": "2h", "345600": "4d", "90": "1m30s", "": "-"}
	for in, want := range cases {
		if got := humanSeconds(in); got != want {
			t.Errorf("humanSeconds(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	if got := prettyJSON(`{"a":1}`); got != "{\n  \"a\": 1\n}" {
		t.Errorf("got %q", got)
	}
	if got := prettyJSON("plain text"); got != "plain text" {
		t.Errorf("non-JSON must be kept, got %q", got)
	}
}

func TestQueueFieldsDLQ(t *testing.T) {
	q := newQueue("https://sqs.us-east-1.amazonaws.com/123/orders.fifo")
	q.attrs = map[string]string{string(types.QueueAttributeNameRedrivePolicy): `{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:123:orders-dlq.fifo","maxReceiveCount":5}`}
	if !q.fifo() || q.name != "orders.fifo" {
		t.Fatalf("bad queue parsing: %+v", q)
	}
	found := false
	for _, f := range queueFields(q) {
		if f.Name == "Max receive count" && f.Value == "5" {
			found = true
		}
	}
	if !found {
		t.Error("redrive policy not rendered")
	}
}
