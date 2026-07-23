package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/vault"
)

func TestDetectPDFBrowserUsesSupportedCommandsInOrder(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "chromium")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var tried []string
	browser, err := detectPDFBrowserWith(func(command string) (string, error) {
		tried = append(tried, command)
		if command == "chromium-browser" {
			return executable, nil
		}
		return "", os.ErrNotExist
	})
	if err != nil {
		t.Fatal(err)
	}
	if browser.Name != "Chromium" || browser.Path != executable {
		t.Fatalf("browser = %+v", browser)
	}
	if strings.Join(tried, ",") != "chromium,chromium-browser" {
		t.Fatalf("commands = %v", tried)
	}
}

func TestDetectPDFBrowserReportsAbsence(t *testing.T) {
	_, err := detectPDFBrowserWith(func(string) (string, error) {
		return "", os.ErrNotExist
	})
	if err == nil {
		t.Fatal("browser detection succeeded")
	}
}

func TestRunInternalPDFWorkerRejectsInvalidArgumentsBeforeStartingBrowser(t *testing.T) {
	err := runInternalPDFWorker([]string{
		"--browser", "/bin/false",
		"--margin-top", "0.4",
		"--margin-right", "1",
		"--margin-bottom", "1",
		"--margin-left", "1",
	}, strings.NewReader("<html></html>"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "marge") {
		t.Fatalf("error = %v", err)
	}
}

func TestPDFWorkerArgumentsKeepBooleanValueAttached(t *testing.T) {
	document := vault.PDFDocument{
		Margins:     vault.PDFPageMargins{Top: 10, Right: 10, Bottom: 10, Left: 10},
		PageNumbers: true,
	}
	args := pdfWorkerArguments(detectedPDFBrowser{Path: "/usr/bin/chromium"}, document)
	if got := args[len(args)-1]; got != "--page-numbers=true" {
		t.Fatalf("last argument = %q", got)
	}
}

func TestRenderPDFInWorkerValidatesOutputAndHonoursCancellation(t *testing.T) {
	script := filepath.Join(t.TempDir(), "pdf-worker")
	content := `#!/bin/sh
case "$PDF_TEST_MODE" in
  valid) printf '%%PDF-1.7\nresult' ;;
  invalid) printf 'not a pdf' ;;
  fail) exit 1 ;;
  wait) sleep 10 ;;
esac
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	document := vault.PDFDocument{
		HTML:        []byte("<!doctype html><p>Note</p>"),
		Margins:     vault.PDFPageMargins{Top: 10, Right: 10, Bottom: 10, Left: 10},
		PageNumbers: true,
	}
	browser := detectedPDFBrowser{Name: "Test", Path: "/bin/false"}

	t.Run("valid", func(t *testing.T) {
		t.Setenv("PDF_TEST_MODE", "valid")
		pdf, err := renderPDFInWorker(t.Context(), script, browser, document)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
			t.Fatalf("output = %q", pdf)
		}
	})
	t.Run("non pdf", func(t *testing.T) {
		t.Setenv("PDF_TEST_MODE", "invalid")
		if _, err := renderPDFInWorker(t.Context(), script, browser, document); err == nil {
			t.Fatal("non-PDF output was accepted")
		}
	})
	t.Run("worker error", func(t *testing.T) {
		t.Setenv("PDF_TEST_MODE", "fail")
		if _, err := renderPDFInWorker(t.Context(), script, browser, document); err == nil {
			t.Fatal("worker failure was accepted")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		t.Setenv("PDF_TEST_MODE", "wait")
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := renderPDFInWorker(ctx, script, browser, document); err == nil {
			t.Fatal("timeout was accepted")
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("worker group was not killed promptly: %s", elapsed)
		}
	})
}

func TestPDFWorkerWithRealChromium(t *testing.T) {
	if os.Getenv("NOTEAULT_PDF_INTEGRATION") != "1" {
		t.Skip("set NOTEAULT_PDF_INTEGRATION=1 to run the Chromium integration test")
	}
	browser, err := detectPDFBrowser()
	if err != nil {
		t.Skip(err)
	}
	service, err := vault.New(vault.Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	)
	if err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(service.Root(), "assets", "pixel.png")
	if err := os.WriteFile(assetPath, png, 0o644); err != nil {
		t.Fatal(err)
	}
	note, err := service.CreateNote("", "Intégration PDF", "")
	if err != nil {
		t.Fatal(err)
	}
	note.Content = "| A | B |\n|---|---|\n| 1 | 2 |\n\n- [x] Tâche\n\n```go\nfmt.Println(\"ok\")\n```\n\n![Pixel](assets/pixel.png)"
	if _, err := service.SaveNote(note); err != nil {
		t.Fatal(err)
	}
	document, err := service.BuildNotePDFDocument(note.RelativePath, "classic", false)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	args := pdfWorkerArguments(browser, document)[1:]
	if err := runInternalPDFWorker(args, bytes.NewReader(document.HTML), &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() < 100 || !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatalf("invalid PDF output: %d bytes", output.Len())
	}
}
