package logging

import (
	"bytes"
	"regexp"
	"testing"
)

func TestLoggerUsesRequiredFormat(t *testing.T) {
	buffer := &bytes.Buffer{}
	logger := New(buffer, LevelTrace)

	logger.Information("hello world")

	pattern := regexp.MustCompile(`^\[[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}Z\] - \[Information\] - hello world\n$`)
	if !pattern.Match(buffer.Bytes()) {
		t.Fatalf("unexpected log format: %q", buffer.String())
	}
}

func TestLoggerHonorsMinimumLevel(t *testing.T) {
	buffer := &bytes.Buffer{}
	logger := New(buffer, LevelWarning)

	logger.Information("skip me")
	logger.Warning("keep me")

	if got := buffer.String(); regexp.MustCompile(`keep me`).FindString(got) == "" || regexp.MustCompile(`skip me`).FindString(got) != "" {
		t.Fatalf("unexpected log output: %q", got)
	}
}