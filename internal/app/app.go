// Package app is the root Bubble Tea model: it owns the navigation stack,
// the header/footer chrome, the global prompts (confirm, text input) and the
// pickers used to switch service, profile and region.
package app

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lbenardo/aws-tui/internal/awsconfig"
	"github.com/lbenardo/aws-tui/internal/service"
	"github.com/lbenardo/aws-tui/internal/ui"
)

const (
	headerHeight = 2
	footerHeight = 2
)

// statusTimeout is how long a footer message stays visible.
var statusTimeout = 6 * time.Second

var (
	keyQuit    = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
	keyBack    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyService = key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "service"))
	keyProfile = key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "profile"))
	keyRegion  = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "region"))
)

type (
	clearStatusMsg struct{ id int }
	// openServiceMsg opens the root view of a service above the home screen.
	openServiceMsg struct{ svc service.Service }
	// reconfiguredMsg carries a freshly loaded AWS config (profile/region switch).
	reconfiguredMsg struct {
		sctx    service.Context
		profile string
		region  string
		err     error
	}
)

// loadConfig is a variable so tests can switch profile/region offline.
var loadConfig = awsconfig.Load

// Options configures the app.
type Options struct {
	Context *service.Context
	// Initial service opened on start (e.g. `aws-tui s3`), optional.
	Initial *service.Service
	// Profile and Region hold what was chosen explicitly (flags or P/R);
	// empty means "use the SDK defaults".
	Profile string
	Region  string
}

// Model is the root model of the application.
type Model struct {
	sctx    *service.Context
	profile string
	region  string
	current *service.Service // service whose root is stack[1], if any

	stack  []ui.View
	width  int
	height int
	help   help.Model

	status    string
	statusErr bool
	statusID  int

	confirm *ui.ConfirmMsg
	prompt  *ui.PromptMsg
	input   textinput.Model
	picker  *picker
}

func New(opts Options) *Model {
	in := textinput.New()
	in.PromptStyle = ui.PromptStyle
	m := &Model{sctx: opts.Context, profile: opts.Profile, region: opts.Region, help: help.New(), input: in}
	m.stack = []ui.View{newHome()}
	if opts.Initial != nil {
		svc := *opts.Initial
		m.current = &svc
		m.stack = append(m.stack, svc.New(m.sctx))
	}
	return m
}

func (m *Model) top() ui.View { return m.stack[len(m.stack)-1] }

func (m *Model) bodySize() (int, int) {
	return m.width, max(m.height-headerHeight-footerHeight, 1)
}

func (m *Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.stack))
	for _, v := range m.stack {
		cmds = append(cmds, v.Init())
	}
	return tea.Batch(cmds...)
}

func (m *Model) setStatus(text string, isErr bool) tea.Cmd {
	m.statusID++
	m.status, m.statusErr = text, isErr
	id := m.statusID
	return tea.Tick(statusTimeout, func(time.Time) tea.Msg { return clearStatusMsg{id: id} })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.input.Width = max(msg.Width-30, 10)
		w, h := m.bodySize()
		for _, v := range m.stack {
			v.SetSize(w, h)
		}
		return m, nil

	case ui.PushMsg:
		msg.View.SetSize(m.bodySize())
		m.stack = append(m.stack, msg.View)
		return m, msg.View.Init()

	case ui.PopMsg:
		m.pop()
		return m, nil

	case openServiceMsg:
		return m, m.switchTo(msg.svc)

	case ui.StatusMsg:
		return m, m.setStatus(msg.Text, false)

	case ui.ErrorMsg:
		return m, m.setStatus(msg.Err.Error(), true)

	case ui.CopyMsg:
		return m, copyToClipboard(msg.Text)

	case ui.TaskResultMsg:
		if msg.Err != nil {
			return m, m.setStatus(msg.Err.Error(), true)
		}
		cmd := m.setStatus(msg.Status, false)
		if msg.Refresh {
			cmd = tea.Batch(cmd, m.updateTop(ui.RefreshMsg{}))
		}
		return m, cmd

	case reconfiguredMsg:
		return m, m.applyConfig(msg)

	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
		return m, nil

	case ui.ConfirmMsg:
		m.prompt = nil
		m.confirm = &msg
		return m, nil

	case ui.PromptMsg:
		m.confirm = nil
		m.prompt = &msg
		m.input.Prompt = msg.Label + " "
		m.input.SetValue(msg.Value)
		m.input.CursorEnd()
		return m, m.input.Focus()

	case ui.RefreshMsg:
		return m, m.updateTop(msg)

	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}

	// Async results are broadcast: each view only reacts to its own messages,
	// so a load that finishes after the user navigated away is not lost.
	cmds := make([]tea.Cmd, 0, len(m.stack))
	for i, v := range m.stack {
		var cmd tea.Cmd
		m.stack[i], cmd = v.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) updateTop(msg tea.Msg) tea.Cmd {
	v, cmd := m.top().Update(msg)
	m.stack[len(m.stack)-1] = v
	return cmd
}

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
	if len(m.stack) == 1 {
		m.current = nil
	}
}

// switchTo replaces everything above the home screen with the root view of
// the given service.
func (m *Model) switchTo(svc service.Service) tea.Cmd {
	m.current = &svc
	v := svc.New(m.sctx)
	v.SetSize(m.bodySize())
	m.stack = append(m.stack[:1], v)
	return v.Init()
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}

	if m.confirm != nil {
		switch strings.ToLower(msg.String()) {
		case "y":
			cmd := m.confirm.OnConfirm
			m.confirm = nil
			return cmd
		case "n", "esc", "q":
			m.confirm = nil
		}
		return nil
	}

	if m.prompt != nil {
		switch msg.String() {
		case "enter":
			p := m.prompt
			m.prompt = nil
			m.input.Blur()
			return p.OnSubmit(strings.TrimSpace(m.input.Value()))
		case "esc":
			m.prompt = nil
			m.input.Blur()
			return nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd
	}

	if m.picker != nil {
		done, cmd := m.picker.update(msg)
		if done {
			m.picker = nil
		}
		return cmd
	}

	if c, ok := m.top().(ui.InputCapturer); ok && c.CapturingInput() {
		return m.updateTop(msg)
	}

	switch {
	case key.Matches(msg, keyService):
		m.picker = m.servicePicker()
		return nil
	case key.Matches(msg, keyProfile):
		m.picker = m.profilePicker()
		return nil
	case key.Matches(msg, keyRegion):
		m.picker = m.regionPicker()
		return nil
	case key.Matches(msg, keyQuit):
		return tea.Quit
	case key.Matches(msg, keyBack):
		if b, ok := m.top().(ui.BackHandler); ok && b.HandleBack() {
			return nil
		}
		m.pop()
		return nil
	}
	return m.updateTop(msg)
}

// ---- pickers ---------------------------------------------------------------

func (m *Model) servicePicker() *picker {
	var items []pickerItem
	for _, s := range service.All() {
		items = append(items, pickerItem{Value: s.ID, Title: s.Name, Desc: s.Description})
	}
	current := ""
	if m.current != nil {
		current = m.current.ID
	}
	return newPicker(":", "service (tab completes, enter switches, esc cancels)", items, current, false,
		func(id string) tea.Cmd {
			svc, _ := service.Get(id)
			return func() tea.Msg { return openServiceMsg{svc: svc} }
		})
}

func (m *Model) profilePicker() *picker {
	var items []pickerItem
	for _, p := range awsconfig.ListProfiles() {
		desc := p.Source
		if p.Region != "" {
			desc += " · " + p.Region
		}
		items = append(items, pickerItem{Value: p.Name, Desc: desc})
	}
	return newPicker("profile: ", "filter profiles (enter switches, esc cancels)", items, m.sctx.Profile, true,
		func(profile string) tea.Cmd { return m.reconfigure(profile, m.region) })
}

func (m *Model) regionPicker() *picker {
	items := make([]pickerItem, len(awsconfig.Regions))
	for i, r := range awsconfig.Regions {
		items[i] = pickerItem{Value: r.Code, Title: r.Name}
	}
	return newPicker("region: ", "filter regions (enter switches, esc cancels)", items, m.sctx.Region, true,
		func(region string) tea.Cmd { return m.reconfigure(m.profile, region) })
}

// reconfigure loads the AWS config for a profile/region in the background.
// An empty region keeps the profile's own region.
func (m *Model) reconfigure(profile, region string) tea.Cmd {
	return tea.Batch(
		ui.Status("Loading AWS config (profile %s, region %s)…", awsconfig.ProfileName(profile), orDefault(region, "profile default")),
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cfg, err := loadConfig(ctx, profile, region)
			return reconfiguredMsg{
				sctx:    service.Context{AWS: cfg, Profile: awsconfig.ProfileName(profile), Region: cfg.Region},
				profile: profile,
				region:  region,
				err:     err,
			}
		},
	)
}

// applyConfig swaps the AWS config and reopens the current service, since
// its views hold clients bound to the previous credentials/region.
func (m *Model) applyConfig(msg reconfiguredMsg) tea.Cmd {
	if msg.err != nil {
		return m.setStatus("switching config: "+msg.err.Error(), true)
	}
	*m.sctx = msg.sctx
	m.profile, m.region = msg.profile, msg.region
	status := m.setStatus("Using profile "+m.sctx.Profile+" in "+m.sctx.Region, false)
	if m.current == nil {
		m.stack = m.stack[:1]
		return status
	}
	return tea.Batch(status, m.switchTo(*m.current))
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---- rendering -------------------------------------------------------------

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	_, bodyH := m.bodySize()
	content := m.top().View()
	if m.picker != nil {
		content = m.picker.view(m.width) + "\n" + content
	}
	body := lipgloss.NewStyle().Width(m.width).Height(bodyH).MaxHeight(bodyH).Render(content)
	return lipgloss.JoinVertical(lipgloss.Left, m.header(), body, m.footer())
}

func (m *Model) header() string {
	logo := ui.LogoStyle.Render("aws-tui")
	hint := ui.MutedStyle.Render
	ctx := hint(" profile: ") + m.sctx.Profile + hint(" [P]") +
		hint("  region: ") + m.sctx.Region + hint(" [R]")

	crumbs := make([]string, len(m.stack))
	for i, v := range m.stack {
		crumbs[i] = v.Title()
	}
	trail := hint(strings.Join(crumbs[:len(crumbs)-1], " › "))
	if len(crumbs) > 1 {
		trail += hint(" › ")
	}
	trail += ui.TitleStyle.Render(crumbs[len(crumbs)-1])

	line := lipgloss.NewStyle().MaxWidth(m.width).Render(logo + ctx + "   " + trail)
	return line + "\n"
}

func (m *Model) footer() string {
	var line string
	switch {
	case m.confirm != nil:
		line = ui.PromptStyle.Render(m.confirm.Prompt) + ui.MutedStyle.Render(" [y/N]")
	case m.prompt != nil:
		line = m.input.View() + ui.MutedStyle.Render("  (enter to confirm, esc to cancel)")
	case m.status != "" && m.statusErr:
		line = ui.ErrorStyle.Render("✗ " + m.status)
	case m.status != "":
		line = ui.OKStyle.Render("• ") + m.status
	}
	line = lipgloss.NewStyle().MaxWidth(m.width).Render(line)

	if m.picker != nil {
		return line + "\n" + ui.MutedStyle.Render("↑/↓ select • tab complete • enter switch • esc cancel")
	}
	keys := append([]key.Binding{}, m.top().KeyBindings()...)
	if len(m.stack) > 1 {
		keys = append(keys, keyBack)
	}
	keys = append(keys, keyService, keyQuit)
	return line + "\n" + m.help.ShortHelpView(keys)
}
