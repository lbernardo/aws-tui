package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbenardo/aws-tui/internal/service"
	"github.com/lbenardo/aws-tui/internal/ui"
)

func init() {
	statusTimeout = time.Millisecond
	service.Register(service.Service{
		ID: "fake", Name: "Fake", Description: "fake service for tests",
		New: func(*service.Context) ui.View {
			return ui.NewTable(ui.TableConfig{
				Title:   "Things",
				Columns: []ui.Column{{Title: "Name"}},
				Load: func(context.Context) ([]ui.Row, error) {
					return []ui.Row{{ID: "id-alpha", Cells: []string{"alpha"}}, {ID: "id-beta", Cells: []string{"beta"}}}, nil
				},
				Tags: func(_ context.Context, r ui.Row) (map[string]string, error) {
					return map[string]string{"env": "prod", "owner": r.ID}, nil
				},
			})
		},
	})
}

// drain runs a command and feeds resulting messages back into the model,
// skipping ticks so the test does not block on timers.
func drain(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case nil:
		default:
			if strings.Contains(strings.ToLower(fmt.Sprintf("%T", msg)), "tick") {
				continue
			}
			if _, ok := msg.(clearStatusMsg); ok {
				continue // keep status visible for assertions
			}
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}

func TestNavigation(t *testing.T) {
	m := New(Options{Context: &service.Context{Profile: "test", Region: "us-east-1"}})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	drain(t, m, m.Init())

	view := m.View()
	for _, want := range []string{"aws-tui", "profile: test", "Services", "Fake"} {
		if !strings.Contains(view, want) {
			t.Fatalf("home view missing %q:\n%s", want, view)
		}
	}

	// Select "Fake" (filter to it, then enter).
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	drain(t, m, cmd)
	for _, r := range "fake" {
		_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		drain(t, m, cmd)
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // leave filter mode
	drain(t, m, cmd)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // open service
	drain(t, m, cmd)

	view = m.View()
	if !strings.Contains(view, "Services › Things") || !strings.Contains(view, "beta") {
		t.Fatalf("service view not shown:\n%s", view)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	drain(t, m, cmd)
	if len(m.stack) != 1 {
		t.Fatalf("esc should pop back to home, stack=%d", len(m.stack))
	}
}

func typeKeys(t *testing.T, m *Model, s string) {
	t.Helper()
	for _, r := range s {
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		drain(t, m, cmd)
	}
}

func TestPalette(t *testing.T) {
	m := New(Options{Context: &service.Context{Profile: "test", Region: "us-east-1"}})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	drain(t, m, m.Init())

	// Cancel with esc keeps the current screen.
	typeKeys(t, m, ":fa")
	if m.picker == nil || !strings.Contains(m.View(), "fake service for tests") {
		t.Fatalf("palette should be open and show the match:\n%s", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker != nil || len(m.stack) != 1 {
		t.Fatalf("esc should close the palette without switching (stack=%d)", len(m.stack))
	}

	// Filter with no match: enter does nothing.
	typeKeys(t, m, ":zzz")
	if !strings.Contains(m.View(), "no matches") {
		t.Fatalf("expected empty match message:\n%s", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.picker == nil || len(m.stack) != 1 {
		t.Fatal("enter without match should keep the palette open")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Switch with enter replaces the stack above home.
	typeKeys(t, m, ":fake")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drain(t, m, cmd)
	if m.picker != nil || len(m.stack) != 2 || m.top().Title() != "Things" {
		t.Fatalf("expected to switch to Fake service, stack=%d", len(m.stack))
	}

	// ":" inside a filter is just text, not the palette.
	typeKeys(t, m, "/:")
	if m.picker != nil {
		t.Fatal("palette must not open while a view captures input")
	}
}

func openFake(t *testing.T) *Model {
	t.Helper()
	m := New(Options{Context: &service.Context{Profile: "test", Region: "us-east-1"}})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	drain(t, m, m.Init())
	typeKeys(t, m, ":fake")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drain(t, m, cmd)
	return m
}

func TestCopyID(t *testing.T) {
	var copied string
	writeClipboard = func(s string) error { copied = s; return nil }
	m := openFake(t)

	typeKeys(t, m, "y")
	if copied != "id-alpha" {
		t.Fatalf("copied %q, want id-alpha", copied)
	}
	if !strings.Contains(m.View(), "Copied id-alpha") {
		t.Fatalf("status not shown:\n%s", m.View())
	}
}

func TestTags(t *testing.T) {
	m := openFake(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	drain(t, m, cmd)
	typeKeys(t, m, "t")
	view := m.View()
	for _, want := range []string{"Tags: id-beta", "env", "prod", "owner"} {
		if !strings.Contains(view, want) {
			t.Fatalf("tags view missing %q:\n%s", want, view)
		}
	}
}

func TestSwitchRegionAndProfile(t *testing.T) {
	var gotProfile, gotRegion string
	loadConfig = func(_ context.Context, profile, region string) (aws.Config, error) {
		gotProfile, gotRegion = profile, region
		if region == "" {
			region = "us-east-1"
		}
		return aws.Config{Region: region}, nil
	}
	m := openFake(t)

	// Cancel keeps everything as is.
	typeKeys(t, m, "Rsa-east")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker != nil || m.sctx.Region != "us-east-1" {
		t.Fatal("esc must cancel the region switch")
	}

	typeKeys(t, m, "Rsa-east")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drain(t, m, cmd)
	if m.sctx.Region != "sa-east-1" || gotRegion != "sa-east-1" {
		t.Fatalf("region not switched: ctx=%s loaded=%s", m.sctx.Region, gotRegion)
	}
	if len(m.stack) != 2 || m.top().Title() != "Things" {
		t.Fatalf("current service should be reopened, stack=%d", len(m.stack))
	}

	// A profile typed by hand (not in ~/.aws) is accepted; region is kept.
	typeKeys(t, m, "Pmy-custom-profile")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	drain(t, m, cmd)
	if gotProfile != "my-custom-profile" || gotRegion != "sa-east-1" || m.sctx.Profile != "my-custom-profile" {
		t.Fatalf("profile switch: profile=%s region=%s ctx=%+v", gotProfile, gotRegion, m.sctx)
	}
	if !strings.Contains(m.View(), "profile: my-custom-profile") {
		t.Fatalf("header not updated:\n%s", m.View())
	}
}
