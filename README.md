# aws-tui

A terminal UI for browsing AWS services, written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

Available services: **S3**, **EC2**, **VPC** and **SQS**.

## Installation

```bash
go install github.com/lbernardo/aws-tui@latest
```

## Usage

```bash
aws-tui                                       # services list
aws-tui s3                                    # open S3 directly
aws-tui --profile prod --region sa-east-1 s3
```

Credentials follow the standard AWS SDK chain (`AWS_PROFILE`, `AWS_REGION`, `~/.aws/config`, SSO, etc.).

### Global shortcuts

| Key          | Action                                                                                  |
|--------------|-----------------------------------------------------------------------------------------|
| `↑/↓`, `k/j` | navigate                                                                                |
| `enter`      | open                                                                                    |
| `/`          | filter (`esc` clears)                                                                   |
| `r`          | refresh                                                                                 |
| `esc`        | go back                                                                                 |
| `:`          | switch service (filterable; `tab` completes, `enter` switches, `esc` cancels)           |
| `P`          | switch profile (lists `~/.aws/config` and `~/.aws/credentials`; accepts a typed name)   |
| `R`          | switch region (lists the commercial regions; accepts a typed code)                      |
| `y`          | copy the resource ID to the clipboard (e.g. `i-0abc…`, queue URL, `s3://bucket/key`)    |
| `t`          | view the resource tags                                                                  |
| `q`          | quit                                                                                    |

When you switch profile or region, the current service is reopened using the new credentials or region. A region chosen with `R` or `--region` is kept across profile switches. Otherwise the profile's own region applies.

### S3

- **Buckets**: `c` create, `x` delete (the bucket must be empty), `enter` open.
- **Objects**: folder (prefix) navigation, `d` download, `u` upload, `n` new folder, `x` delete file, `enter` details.
- **Object details**: metadata, a presigned URL (valid for 1h), and `d` download.

### EC2

Uses the selected region.

- **Instances**: `s` start, `S` stop, `b` reboot, `x` terminate. Stop, reboot and terminate ask for confirmation. `enter` opens the details (network, security groups, volumes, IAM, tags), where the same actions are available.
- **Security Groups**: list with rule counts. `enter` shows the inbound and outbound rules.
- **Volumes**: EBS volumes with the attached instance. `enter` shows the details.

### VPC

- **VPCs**: `enter` opens a menu scoped to that VPC (details, plus subnets, route tables, internet gateways and NAT gateways of that VPC only).
- **Subnets**, **Route Tables** (routes shown as `destination → target`, with blackhole routes highlighted), **Internet Gateways** and **NAT Gateways**: list and details.
- The VPC service is read-only.

### SQS

- **Queues**: list with available, in-flight and delayed messages, visibility timeout, retention and DLQ.
  - Actions: `c` create (a `.fifo` suffix creates a FIFO queue with content-based deduplication), `s` send a message, `m` view messages, `p` purge, `x` delete.
  - `enter` shows the queue attributes (DLQ, encryption, policies).
- **Messages**: peeks at up to 50 messages using a visibility timeout of 0, so they stay available to consumers.
  - `enter` shows the body (formatted JSON) and attributes. `x` deletes the message.
  - Peeking increments each message's *receive count*. Peeking many times can move messages to the DLQ, so for queues that have a DLQ the app asks for confirmation first.

## Architecture

```
main.go                      flags, AWS config loading and program start
internal/
  app/                       root model: navigation stack, header, footer, prompts
    picker.go                filterable picker used by ":", "P" and "R"
    clipboard.go             copy to clipboard (system clipboard, OSC 52 fallback)
  ui/                        View contract, messages and generic views
    view.go                  View interface (+ InputCapturer, BackHandler)
    msgs.go                  Push, Pop, Status, Error, Confirm, Prompt, Task, Refresh, Copy
    table.go                 TableView: async list with filter, actions, copy ID and tags
    detail.go                DetailView: attributes of a single resource
    menu.go                  Menu: list of sub-screens (resource types)
    tags.go                  TagsView: key/value list of a resource's tags
  service/                   service registry (Register / Get / All)
  awsconfig/                 aws.Config loading, profiles and regions
  ec2util/                   helpers for services built on the EC2 API (tags)
  services/
    services.go              blank imports of every service
    s3/  ec2/  vpc/  sqs/    service implementations
```

Navigation is a stack of `ui.View`:
- A view opens another with `ui.Push(v)`, and `esc` goes back.
- Operations that change something run through `ui.Task(...)`, which shows progress in the footer and reloads the view when it finishes.
- Confirmations and text input use `ui.Confirm` and `ui.Prompt`. The app handles both.

## Adding a new service

1. Create `internal/services/<name>/<name>.go` and register the service in `init`:

```go
package sns

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbernardo/aws-tui/internal/service"
	"github.com/lbernardo/aws-tui/internal/ui"
)

func init() {
	service.Register(service.Service{
		ID:          "sns",
		Name:        "SNS",
		Description: "Simple Notification Service",
		New: func(ctx *service.Context) ui.View {
			api := sns.NewFromConfig(ctx.AWS)
			return ui.NewTable(ui.TableConfig{
				Title:   "Topics",
				Columns: []ui.Column{{Title: "ARN"}},
				Load: func(ctx context.Context) ([]ui.Row, error) {
					out, err := api.ListTopics(ctx, &sns.ListTopicsInput{})
					if err != nil {
						return nil, err
					}
					var rows []ui.Row
					for _, t := range out.Topics {
						arn := aws.ToString(t.TopicArn)
						rows = append(rows, ui.Row{ID: arn, Cells: []string{arn}, Data: arn})
					}
					return rows, nil
				},
				OnSelect: func(r ui.Row) tea.Cmd { /* ui.Push(newTopicView(...)) */ return nil },
				Actions: []ui.Action{
					ui.NewAction("p", "publish", func(r *ui.Row) tea.Cmd { /* ui.Prompt + ui.Task */ return nil }),
				},
			})
		},
	})
}
```

2. Add the blank import to `internal/services/services.go`:

```go
import (
	_ "github.com/lbernardo/aws-tui/internal/services/ec2"
	_ "github.com/lbernardo/aws-tui/internal/services/s3"
	_ "github.com/lbernardo/aws-tui/internal/services/sns"
	_ "github.com/lbernardo/aws-tui/internal/services/sqs"
	_ "github.com/lbernardo/aws-tui/internal/services/vpc"
)
```

3. Run `go get github.com/aws/aws-sdk-go-v2/service/sns`. The service now shows up on the home screen and as `aws-tui sns`.

**Copy ID (`y`) and tags (`t`).** To enable `y`, fill `ui.Row.ID` (or `DetailConfig.ID` for detail views). To enable `t`, fill `TableConfig.Tags` (or `DetailConfig.Tags`). For services built on the EC2 API, `ec2util.RowTags(api)` and `ec2util.IDTags(api, id)` already load the tags.

**Reserved keys.** The app and the generic views use these keys, so don't use them in `Actions`: `enter`, `/`, `r`, `y`, `t`, `:`, `P`, `R`, `esc`, `q`.

**Create clients inside `New`.** On a profile or region switch, the app rebuilds the current service's root view with the new `service.Context`. A client created outside `New` would keep the old credentials and region.

**Custom screens.** If you need a screen that doesn't fit the table/detail pattern, implement the `ui.View` interface.

## License

[MIT](LICENSE)
