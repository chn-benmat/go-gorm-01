package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestConsoleRecoversFromInvalidMenuAndExits(t *testing.T) {
	var output bytes.Buffer
	c := newConsole(nil, strings.NewReader("abc\n-1\n99\n0\n"), &output)
	if err := c.run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "กรุณากรอกจำนวนเต็ม") || !strings.Contains(output.String(), "ไม่มีเมนูนี้") {
		t.Fatalf("missing validation messages: %s", output.String())
	}
}

func TestConsoleStopsAtEOF(t *testing.T) {
	c := newConsole(nil, strings.NewReader(""), io.Discard)
	if err := c.run(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestConsoleStopsAtInputError(t *testing.T) {
	c := newConsole(nil, strings.NewReader(strings.Repeat("x", 65*1024)), io.Discard)
	if err := c.run(); !errors.Is(err, errConsoleInput) {
		t.Fatalf("expected input error, got %v", err)
	}
}

func TestConsoleReadsThaiAndRejectsNegativeID(t *testing.T) {
	c := newConsole(nil, strings.NewReader("  สมชาย ใจดี  \r\n-1\n"), io.Discard)
	value, err := c.text("ชื่อ")
	if err != nil || value != "สมชาย ใจดี" {
		t.Fatalf("Thai input: %q, %v", value, err)
	}
	if _, err := c.id("ID"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative ID accepted: %v", err)
	}
}
