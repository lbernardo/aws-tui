package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbernardo/aws-tui/internal/app"
	"github.com/lbernardo/aws-tui/internal/awsconfig"
	"github.com/lbernardo/aws-tui/internal/service"
	_ "github.com/lbernardo/aws-tui/internal/services"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "aws-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	profile := flag.String("profile", "", "AWS profile (default: $AWS_PROFILE or \"default\")")
	region := flag.String("region", "", "AWS region (default: $AWS_REGION or the profile's region)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Println("aws-tui", version)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := awsconfig.Load(ctx, *profile, *region)
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	sctx := &service.Context{AWS: cfg, Profile: awsconfig.ProfileName(*profile), Region: cfg.Region}

	opts := app.Options{Context: sctx, Profile: *profile, Region: *region}
	if id := flag.Arg(0); id != "" {
		svc, ok := service.Get(id)
		if !ok {
			return fmt.Errorf("unknown service %q (available: %s)", id, serviceIDs())
		}
		opts.Initial = &svc
	}

	_, err = tea.NewProgram(app.New(opts), tea.WithAltScreen()).Run()
	return err
}

func serviceIDs() string {
	var ids []string
	for _, s := range service.All() {
		ids = append(ids, s.ID)
	}
	return strings.Join(ids, ", ")
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: aws-tui [flags] [service]\n\nServices: %s\n\nFlags:\n", serviceIDs())
	flag.PrintDefaults()
}
