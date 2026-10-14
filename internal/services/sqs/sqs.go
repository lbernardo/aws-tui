// Package sqs exposes Amazon SQS queues and messages.
package sqs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbernardo/aws-tui/internal/service"
	"github.com/lbernardo/aws-tui/internal/ui"
)

func init() {
	service.Register(service.Service{
		ID:          "sqs",
		Name:        "SQS",
		Description: "Simple Queue Service: queues and messages",
		New: func(ctx *service.Context) ui.View {
			return newQueuesView(sqs.NewFromConfig(ctx.AWS))
		},
	})
}

// attrConcurrency bounds parallel GetQueueAttributes calls.
const attrConcurrency = 10

type queue struct {
	url   string
	name  string
	attrs map[string]string
}

func newQueue(url string) queue { return queue{url: url, name: path.Base(url)} }

func (q queue) fifo() bool { return strings.HasSuffix(q.name, ".fifo") }

func (q queue) attr(name types.QueueAttributeName) string { return q.attrs[string(name)] }

func newQueuesView(api *sqs.Client) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Queues",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Type", Width: 8},
			{Title: "Available", Width: 9},
			{Title: "In flight", Width: 9},
			{Title: "Delayed", Width: 7},
			{Title: "Visibility", Width: 10},
			{Title: "Retention", Width: 9},
			{Title: "DLQ", Width: 3},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) { return listQueues(ctx, api) },
		OnSelect: func(r ui.Row) tea.Cmd {
			return ui.Push(newQueueView(api, r.Data.(queue)))
		},
		Actions: append(
			[]ui.Action{ui.NewAction("c", "create", func(*ui.Row) tea.Cmd { return promptCreateQueue(api) })},
			queueActions(api, func(r *ui.Row) (queue, bool) {
				if r == nil {
					return queue{}, false
				}
				return r.Data.(queue), true
			})...,
		),
		Tags: func(ctx context.Context, r ui.Row) (map[string]string, error) {
			return queueTags(ctx, api, r.ID)
		},
		EmptyText: "No queues in this region. Press c to create one.",
	})
}

func listQueues(ctx context.Context, api *sqs.Client) ([]ui.Row, error) {
	var queues []queue
	p := sqs.NewListQueuesPaginator(api, &sqs.ListQueuesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, u := range page.QueueUrls {
			queues = append(queues, newQueue(u))
		}
	}

	// Fetch attributes in parallel: ListQueues only returns URLs.
	var (
		wg       sync.WaitGroup
		sem      = make(chan struct{}, attrConcurrency)
		mu       sync.Mutex
		firstErr error
	)
	for i := range queues {
		wg.Add(1)
		sem <- struct{}{}
		go func(q *queue) {
			defer func() { <-sem; wg.Done() }()
			attrs, err := getAttributes(ctx, api, q.url)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			q.attrs = attrs
		}(&queues[i])
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	rows := make([]ui.Row, len(queues))
	for i, q := range queues {
		kind := "Standard"
		if q.fifo() {
			kind = "FIFO"
		}
		dlq := "no"
		if q.attr(types.QueueAttributeNameRedrivePolicy) != "" {
			dlq = "yes"
		}
		rows[i] = ui.Row{
			ID: q.url,
			Cells: []string{
				q.name,
				kind,
				ui.OrDash(q.attr(types.QueueAttributeNameApproximateNumberOfMessages)),
				ui.OrDash(q.attr(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible)),
				ui.OrDash(q.attr(types.QueueAttributeNameApproximateNumberOfMessagesDelayed)),
				humanSeconds(q.attr(types.QueueAttributeNameVisibilityTimeout)),
				humanSeconds(q.attr(types.QueueAttributeNameMessageRetentionPeriod)),
				dlq,
			},
			Data: q,
		}
	}
	return rows, nil
}

func getAttributes(ctx context.Context, api *sqs.Client, url string) (map[string]string, error) {
	out, err := api.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(url),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameAll},
	})
	if err != nil {
		return nil, err
	}
	return out.Attributes, nil
}

func queueTags(ctx context.Context, api *sqs.Client, url string) (map[string]string, error) {
	out, err := api.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(url)})
	if err != nil {
		return nil, err
	}
	return out.Tags, nil
}

// queueActions are shared by the queue list (selected row) and queue detail.
func queueActions(api *sqs.Client, target func(*ui.Row) (queue, bool)) []ui.Action {
	act := func(fn func(queue) tea.Cmd) func(*ui.Row) tea.Cmd {
		return func(r *ui.Row) tea.Cmd {
			if q, ok := target(r); ok {
				return fn(q)
			}
			return nil
		}
	}
	return []ui.Action{
		ui.NewAction("m", "messages", act(func(q queue) tea.Cmd { return openMessages(api, q) })),
		ui.NewAction("s", "send", act(func(q queue) tea.Cmd { return promptSend(api, q) })),
		ui.NewAction("p", "purge", act(func(q queue) tea.Cmd {
			return ui.Confirm("Purge ALL messages from "+q.name+"?", ui.Task("Purging "+q.name+"…", true, func(ctx context.Context) (string, error) {
				_, err := api.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: aws.String(q.url)})
				return "Purge requested for " + q.name + " (may take up to 60s)", err
			}))
		})),
		ui.NewAction("x", "delete", act(func(q queue) tea.Cmd {
			return ui.Confirm("Delete queue "+q.name+"? This cannot be undone", ui.Task("Deleting "+q.name+"…", true, func(ctx context.Context) (string, error) {
				_, err := api.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(q.url)})
				return "Queue " + q.name + " deleted", err
			}))
		})),
	}
}

func promptCreateQueue(api *sqs.Client) tea.Cmd {
	return ui.Prompt("New queue name (end with .fifo for FIFO):", "", func(name string) tea.Cmd {
		if name == "" {
			return nil
		}
		return ui.Task("Creating queue "+name+"…", true, func(ctx context.Context) (string, error) {
			in := &sqs.CreateQueueInput{QueueName: aws.String(name)}
			if strings.HasSuffix(name, ".fifo") {
				in.Attributes = map[string]string{
					string(types.QueueAttributeNameFifoQueue):                 "true",
					string(types.QueueAttributeNameContentBasedDeduplication): "true",
				}
			}
			if _, err := api.CreateQueue(ctx, in); err != nil {
				return "", err
			}
			return "Queue " + name + " created", nil
		})
	})
}

func promptSend(api *sqs.Client, q queue) tea.Cmd {
	return ui.Prompt("Message body:", "", func(body string) tea.Cmd {
		if body == "" {
			return nil
		}
		return ui.Task("Sending message to "+q.name+"…", true, func(ctx context.Context) (string, error) {
			in := &sqs.SendMessageInput{QueueUrl: aws.String(q.url), MessageBody: aws.String(body)}
			if q.fifo() {
				in.MessageGroupId = aws.String("aws-tui")
				in.MessageDeduplicationId = aws.String(strconv.FormatInt(time.Now().UnixNano(), 10))
			}
			out, err := api.SendMessage(ctx, in)
			if err != nil {
				return "", err
			}
			return "Message " + aws.ToString(out.MessageId) + " sent to " + q.name, nil
		})
	})
}

func newQueueView(api *sqs.Client, q queue) ui.View {
	return ui.NewDetail(ui.DetailConfig{
		Title: q.name,
		Load: func(ctx context.Context) ([]ui.Field, error) {
			attrs, err := getAttributes(ctx, api, q.url)
			if err != nil {
				return nil, err
			}
			q.attrs = attrs
			return queueFields(q), nil
		},
		Actions: queueActions(api, func(*ui.Row) (queue, bool) { return q, true }),
		ID:      q.url,
		Tags:    func(ctx context.Context) (map[string]string, error) { return queueTags(ctx, api, q.url) },
	})
}

func queueFields(q queue) []ui.Field {
	a := func(n types.QueueAttributeName) string { return ui.OrDash(q.attr(n)) }
	kind := "Standard"
	if q.fifo() {
		kind = "FIFO"
		if q.attr(types.QueueAttributeNameContentBasedDeduplication) == "true" {
			kind += " (content-based deduplication)"
		}
	}
	encryption := "-"
	switch {
	case q.attr(types.QueueAttributeNameKmsMasterKeyId) != "":
		encryption = "SSE-KMS " + q.attr(types.QueueAttributeNameKmsMasterKeyId)
	case q.attr(types.QueueAttributeNameSqsManagedSseEnabled) == "true":
		encryption = "SSE-SQS"
	}

	fields := []ui.Field{
		{Name: "Name", Value: q.name},
		{Name: "URL", Value: q.url},
		{Name: "ARN", Value: a(types.QueueAttributeNameQueueArn)},
		{Name: "Type", Value: kind},
		{Name: "Encryption", Value: encryption},
		{Name: "Created", Value: formatEpoch(q.attr(types.QueueAttributeNameCreatedTimestamp))},
		{Name: "Last modified", Value: formatEpoch(q.attr(types.QueueAttributeNameLastModifiedTimestamp))},
		{},
		{Name: "Messages available", Value: a(types.QueueAttributeNameApproximateNumberOfMessages)},
		{Name: "Messages in flight", Value: a(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible)},
		{Name: "Messages delayed", Value: a(types.QueueAttributeNameApproximateNumberOfMessagesDelayed)},
		{},
		{Name: "Visibility timeout", Value: humanSeconds(q.attr(types.QueueAttributeNameVisibilityTimeout))},
		{Name: "Retention period", Value: humanSeconds(q.attr(types.QueueAttributeNameMessageRetentionPeriod))},
		{Name: "Delivery delay", Value: humanSeconds(q.attr(types.QueueAttributeNameDelaySeconds))},
		{Name: "Receive wait time", Value: humanSeconds(q.attr(types.QueueAttributeNameReceiveMessageWaitTimeSeconds))},
		{Name: "Max message size", Value: humanBytesAttr(q.attr(types.QueueAttributeNameMaximumMessageSize))},
	}

	if rp := q.attr(types.QueueAttributeNameRedrivePolicy); rp != "" {
		var policy struct {
			DeadLetterTargetArn string `json:"deadLetterTargetArn"`
			MaxReceiveCount     any    `json:"maxReceiveCount"`
		}
		if json.Unmarshal([]byte(rp), &policy) == nil {
			fields = append(fields, ui.Field{},
				ui.Field{Name: "Dead-letter queue", Value: policy.DeadLetterTargetArn},
				ui.Field{Name: "Max receive count", Value: fmt.Sprint(policy.MaxReceiveCount)})
		} else {
			fields = append(fields, ui.Field{}, ui.Field{Name: "Redrive policy", Value: rp})
		}
	}
	if p := q.attr(types.QueueAttributeNameRedriveAllowPolicy); p != "" {
		fields = append(fields, ui.Field{Name: "Redrive allow policy", Value: prettyJSON(p)})
	}
	if p := q.attr(types.QueueAttributeNamePolicy); p != "" {
		fields = append(fields, ui.Field{}, ui.Field{Name: "Access policy", Value: prettyJSON(p)})
	}
	return fields
}

// humanSeconds turns an attribute in seconds into "30s", "5m", "4d"...
func humanSeconds(v string) string {
	n, err := strconv.Atoi(v)
	if err != nil {
		return ui.OrDash(v)
	}
	d := time.Duration(n) * time.Second
	switch {
	case n == 0:
		return "0s"
	case n%86400 == 0:
		return fmt.Sprintf("%dd", n/86400)
	case n%3600 == 0:
		return fmt.Sprintf("%dh", n/3600)
	case n%60 == 0:
		return fmt.Sprintf("%dm", n/60)
	case n < 60:
		return fmt.Sprintf("%ds", n)
	default:
		return d.String()
	}
}

func humanBytesAttr(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return ui.OrDash(v)
	}
	return ui.HumanBytes(n)
}

// formatEpoch formats a unix timestamp in seconds (or milliseconds).
func formatEpoch(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return ui.OrDash(v)
	}
	if n > 1e12 {
		return ui.FormatTime(time.UnixMilli(n))
	}
	return ui.FormatTime(time.Unix(n, 0))
}

func prettyJSON(s string) string {
	var buf bytes.Buffer
	if json.Indent(&buf, []byte(s), "", "  ") != nil {
		return s
	}
	return buf.String()
}
