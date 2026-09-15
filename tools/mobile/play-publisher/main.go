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
	if len(os.Args) < 2 || os.Args[1] != "inspect-app" {
		return errors.New("usage: play-publisher inspect-app [--package-name PACKAGE]")
	}
	inspect := flag.NewFlagSet("inspect-app", flag.ContinueOnError)
	inspect.SetOutput(os.Stderr)
	packageName := inspect.String("package-name", defaultPackageName, "Android application package name")
	baseURL := inspect.String("base-url", os.Getenv("PLAY_API_BASE_URL"), "Android Publisher API HTTPS origin")
	if err := inspect.Parse(os.Args[2:]); err != nil {
		return err
	}
	if *packageName == "" {
		return errors.New("--package-name must not be empty")
	}
	token := os.Getenv("PLAY_ACCESS_TOKEN")
	client, err := NewClient(*baseURL, token, nil)
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
