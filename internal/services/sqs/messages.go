package sqs

import (
	"context"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbenardo/aws-tui/internal/ui"
)

const (
	// peekRounds is how many ReceiveMessage calls a peek makes (10 msgs each).
	peekRounds = 5
	// peekMaxMessages caps how many distinct messages a peek returns.
	peekMaxMessages = 50
)

// openMessages peeks at the queue. Receiving a message increments its
// receive count, so when a DLQ is configured we ask first: peeking too many
// times could move messages to the dead-letter queue.
func openMessages(api *sqs.Client, q queue) tea.Cmd {
	view := ui.Push(newMessagesView(api, q))
	if q.attr(types.QueueAttributeNameRedrivePolicy) != "" {
		return ui.Confirm("Peeking increases the receive count and may move messages to the DLQ. Continue?", view)
	}
	return view
}

func newMessagesView(api *sqs.Client, q queue) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Messages",
		Columns: []ui.Column{
			{Title: "Message ID", Width: 36},
			{Title: "Sent", Width: 19},
			{Title: "Receives", Width: 8},
			{Title: "Body"},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			msgs, err := peek(ctx, api, q.url)
			if err != nil {
				return nil, err
			}
			rows := make([]ui.Row, len(msgs))
			for i, m := range msgs {
				rows[i] = ui.Row{
					ID: aws.ToString(m.MessageId),
					Cells: []string{
						aws.ToString(m.MessageId),
						formatEpoch(m.Attributes[string(types.MessageSystemAttributeNameSentTimestamp)]),
						ui.OrDash(m.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)]),
						oneLine(aws.ToString(m.Body)),
					},
					Data: m,
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			m := r.Data.(types.Message)
			return ui.Push(ui.NewDetail(ui.DetailConfig{
				Title: aws.ToString(m.MessageId),
				Load:  func(context.Context) ([]ui.Field, error) { return messageFields(m), nil },
				ID:    aws.ToString(m.MessageId),
				Actions: []ui.Action{
					ui.NewAction("x", "delete", func(*ui.Row) tea.Cmd { return confirmDeleteMessage(api, q, m) }),
				},
			}))
		},
		Actions: []ui.Action{
			ui.NewAction("s", "send", func(*ui.Row) tea.Cmd { return promptSend(api, q) }),
			ui.NewAction("x", "delete", func(r *ui.Row) tea.Cmd {
				if r == nil {
					return nil
				}
				return confirmDeleteMessage(api, q, r.Data.(types.Message))
			}),
		},
		EmptyText: "No messages available (in-flight and delayed messages are not shown).",
	})
}

// peek receives messages with a zero visibility timeout, so they stay
// available for real consumers. Several rounds are made because SQS returns
// a sample of at most 10 messages per call.
func peek(ctx context.Context, api *sqs.Client, url string) ([]types.Message, error) {
	seen := map[string]types.Message{}
	misses := 0
	for round := 0; round < peekRounds && len(seen) < peekMaxMessages && misses < 2; round++ {
		out, err := api.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:                    aws.String(url),
			MaxNumberOfMessages:         10,
			VisibilityTimeout:           0,
			WaitTimeSeconds:             1,
			MessageSystemAttributeNames: []types.MessageSystemAttributeName{types.MessageSystemAttributeNameAll},
			MessageAttributeNames:       []string{"All"},
		})
		if err != nil {
			return nil, err
		}
		fresh := 0
		for _, m := range out.Messages {
			id := aws.ToString(m.MessageId)
			if _, ok := seen[id]; !ok {
				fresh++
			}
			seen[id] = m // keep the latest receipt handle
		}
		if fresh == 0 {
			misses++
		}
	}

	msgs := make([]types.Message, 0, len(seen))
	for _, m := range seen {
		msgs = append(msgs, m)
	}
	sentAt := func(m types.Message) string {
		return m.Attributes[string(types.MessageSystemAttributeNameSentTimestamp)]
	}
	sort.Slice(msgs, func(i, j int) bool {
		a, b := sentAt(msgs[i]), sentAt(msgs[j])
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	return msgs, nil
}

func confirmDeleteMessage(api *sqs.Client, q queue, m types.Message) tea.Cmd {
	id := aws.ToString(m.MessageId)
	return ui.Confirm("Delete message "+id+"?", ui.Task("Deleting message "+id+"…", true, func(ctx context.Context) (string, error) {
		_, err := api.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(q.url), ReceiptHandle: m.ReceiptHandle})
		return "Message " + id + " deleted", err
	}))
}

func messageFields(m types.Message) []ui.Field {
	sys := func(n types.MessageSystemAttributeName) string { return ui.OrDash(m.Attributes[string(n)]) }
	fields := []ui.Field{
		{Name: "Message ID", Value: aws.ToString(m.MessageId)},
		{Name: "Sent", Value: formatEpoch(m.Attributes[string(types.MessageSystemAttributeNameSentTimestamp)])},
		{Name: "First received", Value: formatEpoch(m.Attributes[string(types.MessageSystemAttributeNameApproximateFirstReceiveTimestamp)])},
		{Name: "Receive count", Value: sys(types.MessageSystemAttributeNameApproximateReceiveCount)},
		{Name: "Sender", Value: sys(types.MessageSystemAttributeNameSenderId)},
		{Name: "Body MD5", Value: ui.OrDash(aws.ToString(m.MD5OfBody))},
	}
	if g := m.Attributes[string(types.MessageSystemAttributeNameMessageGroupId)]; g != "" {
		fields = append(fields,
			ui.Field{Name: "Group ID", Value: g},
			ui.Field{Name: "Deduplication ID", Value: sys(types.MessageSystemAttributeNameMessageDeduplicationId)},
			ui.Field{Name: "Sequence number", Value: sys(types.MessageSystemAttributeNameSequenceNumber)})
	}

	if len(m.MessageAttributes) > 0 {
		fields = append(fields, ui.Field{})
		names := make([]string, 0, len(m.MessageAttributes))
		for n := range m.MessageAttributes {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			v := m.MessageAttributes[n]
			value := aws.ToString(v.StringValue)
			if value == "" && v.BinaryValue != nil {
				value = ui.HumanBytes(int64(len(v.BinaryValue))) + " binary"
			}
			fields = append(fields, ui.Field{Name: "attr:" + n, Value: value + ui.MutedStyle.Render("  ("+aws.ToString(v.DataType)+")")})
		}
	}
	return append(fields, ui.Field{}, ui.Field{Name: "Body", Value: prettyJSON(aws.ToString(m.Body))})
}

// oneLine collapses whitespace so a body fits in a table cell.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
