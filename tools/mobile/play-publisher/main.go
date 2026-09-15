package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
)

const defaultPackageName = "com.geoguessme.app"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: play-publisher inspect-app|publish-bundle [flags]")
	}
	switch os.Args[1] {
	case "inspect-app":
		return inspectApp(os.Args[2:])
	case "publish-bundle":
		return publishBundleCommand(os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q; usage: play-publisher inspect-app|publish-bundle [flags]", os.Args[1])
	}
}

func inspectApp(args []string) error {
	inspect := flag.NewFlagSet("inspect-app", flag.ContinueOnError)
	inspect.SetOutput(os.Stderr)
	packageName := inspect.String("package-name", defaultPackageName, "Android application package name")
	baseURL := inspect.String("base-url", os.Getenv("PLAY_API_BASE_URL"), "Android Publisher API HTTPS origin")
	if err := inspect.Parse(args); err != nil {
		return err
	}
	if *packageName == "" {
		return errors.New("--package-name must not be empty")
	}
	client, err := NewClient(*baseURL, os.Getenv("PLAY_ACCESS_TOKEN"), nil)
	if err != nil {
		return err
	}
	application, err := client.GetApplication(context.Background(), *packageName)
	if err != nil {
		return err
	}
	if application.PackageName != *packageName {
		return fmt.Errorf("Play API returned package %q, expected %q", application.PackageName, *packageName)
	}
	return json.NewEncoder(os.Stdout).Encode(application)
}

func publishBundleCommand(args []string) error {
	publish := flag.NewFlagSet("publish-bundle", flag.ContinueOnError)
	publish.SetOutput(os.Stderr)
	options := PublishOptions{}
	publish.StringVar(&options.PackageName, "package-name", defaultPackageName, "Android application package name")
	publish.StringVar(&options.BaseURL, "base-url", os.Getenv("PLAY_API_BASE_URL"), "Android Publisher API HTTPS origin")
	publish.StringVar(&options.BundlePath, "bundle", "", "signed Android App Bundle path")
	publish.StringVar(&options.ManifestPath, "manifest", "", "verified release manifest path")
	publish.StringVar(&options.Track, "track", os.Getenv("PLAY_RELEASE_TRACK"), "Play release track")
	publish.StringVar(&options.Status, "status", envOrDefault("PLAY_RELEASE_STATUS", "completed"), "Play release status")
	if err := publish.Parse(args); err != nil {
		return err
	}
	if options.PackageName == "" {
		return errors.New("--package-name must not be empty")
	}
	client, err := NewClient(options.BaseURL, os.Getenv("PLAY_ACCESS_TOKEN"), nil)
	if err != nil {
		return err
	}
	result, err := publishBundle(context.Background(), client, options)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
